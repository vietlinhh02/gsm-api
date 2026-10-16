package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func webhookSignature(payload []byte, signingKey string) string {
	hasher := hmac.New(sha256.New, []byte(signingKey))
	_, _ = hasher.Write(payload)
	return hex.EncodeToString(hasher.Sum(nil))
}

// RunWebhookWorker delivers queued callbacks until the context is cancelled.
func (a *API) RunWebhookWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.deliverOneWebhook(ctx); err != nil {
				a.Logger.Printf("deliver webhook: %v", err)
			}
		}
	}
}

type webhookDelivery struct {
	ID, Target string
	Payload    []byte
}

func (a *API) deliverOneWebhook(ctx context.Context) error {
	var delivery webhookDelivery
	err := a.Store.DB.QueryRowContext(ctx, `SELECT event_id,payload,target_url
  FROM webhook_events WHERE delivered_at IS NULL AND attempts<7 AND next_attempt_at<=NOW()
  AND (target_url<>'' OR $1<>'') ORDER BY next_attempt_at,event_id LIMIT 1`, a.Config.WebhookURL).
		Scan(&delivery.ID, &delivery.Payload, &delivery.Target)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if delivery.Target == "" {
		delivery.Target = a.Config.WebhookURL
	}
	sendErr := a.sendWebhook(ctx, delivery)
	if err := a.recordWebhookDelivery(ctx, delivery.ID, sendErr); err != nil {
		return err
	}
	return sendErr
}

func (a *API) sendWebhook(ctx context.Context, delivery webhookDelivery) error {
	internal := strings.HasPrefix(delivery.Target, "/webhook-inbox/")
	if internal {
		delivery.Target = a.Config.LocalOrigin + delivery.Target
	}
	request, err := a.webhookRequest(ctx, delivery)
	if err != nil {
		return err
	}
	if !internal {
		authorization, err := a.webhookAuthorization(ctx)
		if err != nil {
			return err
		}
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
	}
	response, err := a.HTTP.Do(request)
	if err != nil {
		return webhookTransportError(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("webhook returned HTTP %d", response.StatusCode)
	}
	return nil
}

func webhookTransportError(err error) error {
	var transport *url.Error
	if errors.As(err, &transport) {
		err = transport.Err
	}
	return fmt.Errorf("POST webhook failed: %w", err)
}

func (a *API) webhookRequest(ctx context.Context, delivery webhookDelivery) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, delivery.Target,
		strings.NewReader(string(delivery.Payload)))
	if err != nil {
		return nil, fmt.Errorf("create webhook POST request: invalid destination")
	}
	request.Header.Set("Content-Type", "application/json")
	var event struct {
		ResourceHref string `json:"resource_href"`
	}
	if err := json.Unmarshal(delivery.Payload, &event); err != nil {
		return nil, fmt.Errorf("decode event environment: %w", err)
	}
	environment := "production"
	if strings.Contains(event.ResourceHref, "/v1/sandbox/") {
		environment = "sandbox"
	}
	request.Header.Set("X-Environment", environment)
	if a.Config.WebhookSignKey != "" {
		request.Header.Set("X-VSF-Signature", webhookSignature(delivery.Payload, a.Config.WebhookSignKey))
	}
	if a.Config.WebhookSignKeySecondary != "" {
		request.Header.Set("X-VSF-Signature-Secondary",
			webhookSignature(delivery.Payload, a.Config.WebhookSignKeySecondary))
	}
	return request, nil
}

func (a *API) recordWebhookDelivery(ctx context.Context, id string, sendErr error) error {
	if sendErr == nil {
		_, err := a.Store.DB.ExecContext(ctx, `UPDATE webhook_events
   SET delivered_at=NOW(),attempts=attempts+1 WHERE event_id=$1`, id)
		return err
	}
	var attempts int
	if err := a.Store.DB.QueryRowContext(ctx, `UPDATE webhook_events
  SET attempts=attempts+1 WHERE event_id=$1 RETURNING attempts`, id).Scan(&attempts); err != nil {
		return err
	}
	if attempts >= 7 {
		return nil
	}
	delay := time.Duration(30*(1<<uint(attempts-1))) * time.Second
	_, err := a.Store.DB.ExecContext(ctx, `UPDATE webhook_events
  SET next_attempt_at=NOW()+$1::interval WHERE event_id=$2`, delay.String(), id)
	return err
}

type webhookTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
}

func (a *API) webhookAuthorization(ctx context.Context) (string, error) {
	if a.Config.WebhookTokenURL == "" {
		return a.Config.WebhookAuth, nil
	}
	a.webhookTokenMu.Lock()
	defer a.webhookTokenMu.Unlock()
	if a.webhookToken != "" && time.Now().Before(a.webhookTokenExpires) {
		return "Bearer " + a.webhookToken, nil
	}
	token, err := a.fetchWebhookToken(ctx)
	if err != nil {
		return "", err
	}
	a.webhookToken = token.AccessToken
	a.webhookTokenExpires = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	return "Bearer " + token.AccessToken, nil
}

func (token webhookTokenResponse) valid() bool {
	return token.AccessToken != "" && strings.EqualFold(token.TokenType, "Bearer") &&
		token.ExpiresIn > 0
}

func (a *API) fetchWebhookToken(ctx context.Context) (webhookTokenResponse, error) {
	var token webhookTokenResponse
	form := url.Values{"grant_type": {"client_credentials"},
		"client_id": {a.Config.WebhookClientID}, "client_secret": {a.Config.WebhookClientSecret}}
	if a.Config.WebhookScope != "" {
		form.Set("scope", a.Config.WebhookScope)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.Config.WebhookTokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return token, fmt.Errorf("create webhook token request: invalid token URL")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := a.HTTP.Do(request)
	if err != nil {
		return token, webhookTransportError(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return token, fmt.Errorf("webhook token endpoint returned HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return token, fmt.Errorf("decode webhook token response: %w", err)
	}
	if !token.valid() {
		return token, fmt.Errorf("webhook token response requires access_token, " +
			"Bearer token_type, and positive expires_in")
	}
	return token, nil
}
