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

func (a *API) updateStatus(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UpdateType  string `json:"update_type"`
		OrderID     string `json:"order_id"`
		Status      string `json:"status"`
		PointStatus string `json:"point_status"`
		PointID     string `json:"point_id"`
	}
	if !a.readJSON(w, r, &input) {
		return
	}
	if input.OrderID == "" {
		writeError(w, http.StatusBadRequest, 400, "order_id is required")
		return
	}
	var err error
	switch input.UpdateType {
	case "order":
		if !validOrderStatus(input.Status) {
			writeError(w, http.StatusBadRequest, 400, "status is not a supported order status")
			return
		}
		err = a.changeOrderStatus(r, input.OrderID, input.Status, nil)
	case "point":
		if input.PointID == "" || !validPointStatus(input.PointStatus) {
			writeError(
				w,
				http.StatusBadRequest,
				400,
				"point_id and a supported point_status are required",
			)
			return
		}
		err = a.changePointStatus(r, input.OrderID, input.PointID, input.PointStatus)
	default:
		writeError(w, http.StatusBadRequest, 400, "update_type must be order or point")
		return
	}
	if err != nil {
		var apiErr *httpError
		if errors.As(err, &apiErr) {
			writeError(w, apiErr.Status, apiErr.Code, apiErr.Message)
			return
		}
		a.dbError(w, "update simulated status", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"message": "ok"}})
}

func (a *API) changeOrderStatus(r *http.Request, orderID, status string, cancelCode any) error {
	tx, err := a.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var payload map[string]any
	var oldStatus string
	err = tx.QueryRowContext(r.Context(), `
		SELECT status,payload FROM orders
		WHERE id=$1 AND environment=$2 AND client_id=$3 FOR UPDATE
	`, orderID, requestEnvironment(r.Context()), clientID(r.Context())).
		Scan(&oldStatus, jsonScanner{target: &payload})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &httpError{Status: http.StatusNotFound, Code: 4031, Message: "order not found"}
		}
		return err
	}
	if oldStatus == "COMPLETED" || oldStatus == "CANCELLED" {
		return &httpError{
			Status:  http.StatusConflict,
			Code:    4013,
			Message: "order cannot be changed in its current status",
		}
	}
	if status == "CANCELLED" && oldStatus == "IN PROCESS" {
		return &httpError{
			Status:  http.StatusConflict,
			Code:    4013,
			Message: "order cannot be cancelled in its current status",
		}
	}
	if oldStatus == status {
		return nil
	}
	payload["status"] = status
	if cancelCode != nil {
		payload["cancel_code"] = cancelCode
	}
	if status == "ASSIGNED" {
		assignSimulatedDriver(payload)
	}
	payload["current_server_time"] = fmt.Sprint(time.Now().Unix())
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode updated order: %w", err)
	}
	_, err = tx.ExecContext(
		r.Context(),
		`UPDATE orders SET status=$1,payload=$2,
		 driver_lock_time=CASE WHEN $1='ASSIGNED' THEN $3 ELSE driver_lock_time END,
		 updated_at=NOW()
		 WHERE id=$4 AND environment=$5 AND client_id=$6`,
		status,
		encoded,
		time.Now().Unix(),
		orderID,
		requestEnvironment(r.Context()),
		clientID(r.Context()),
	)
	if err != nil {
		return err
	}
	if err := a.enqueueWebhookTx(tx,
		r.Context(),
		resourceHref(r, orderID),
		map[string]any{"order_id": orderID, "status": status},
		"order_status_changed",
		status,
		clientID(r.Context()),
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	a.publishOrder(requestEnvironment(r.Context()), clientID(r.Context()), orderID)
	return nil
}

func assignSimulatedDriver(payload map[string]any) {
	payload["driver_lock_time"] = fmt.Sprint(time.Now().Unix())
	payload["driver"] = map[string]any{
		"id": "local-driver", "name": "Simulated Driver", "phone_number": "+84900000000",
		"rating": 5.0, "is_device_active": true,
	}
	mode, _ := payload["travel_mode"].(string)
	payload["vehicle"] = simulatedVehicle(mode)
}

func simulatedVehicle(mode string) map[string]any {
	model, vehicleType, plate := "VF5", "car", "29A-000.00 (TEST)"
	if mode == "Motorcycle" {
		model, vehicleType, plate = "VinFast Feliz S", "motorcycle", "29M1-000.00 (TEST)"
	}
	return map[string]any{
		"model": model, "type": vehicleType, "license_plate": plate,
		"vehicle_color_name": "XANH", "has_camera": false,
	}
}

func (a *API) changePointStatus(r *http.Request, orderID, pointID, status string) error {
	tx, err := a.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var payload map[string]any
	err = tx.QueryRowContext(r.Context(), `
		SELECT payload FROM orders
		WHERE id=$1 AND environment=$2 AND client_id=$3 FOR UPDATE
	`, orderID, requestEnvironment(r.Context()), clientID(r.Context())).
		Scan(jsonScanner{target: &payload})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &httpError{Status: http.StatusNotFound, Code: 4031, Message: "order not found"}
		}
		return err
	}
	if payload["status"] == "COMPLETED" || payload["status"] == "CANCELLED" {
		return &httpError{Status: http.StatusConflict, Code: 4013,
			Message: "order cannot be changed in its current status"}
	}
	pathBytes, err := json.Marshal(payload["path"])
	if err != nil {
		return fmt.Errorf("encode order path: %w", err)
	}
	var points []Point
	if err := json.Unmarshal(pathBytes, &points); err != nil {
		return fmt.Errorf("decode order path: %w", err)
	}
	found := false
	for index := range points {
		if points[index].ID == pointID {
			points[index].Status = status
			if status == "ARRIVED" {
				points[index].ArriveTime = fmt.Sprint(time.Now().Unix())
			}
			if status == "COMPLETED" {
				points[index].ProofOfWork = &GPS{
					Latitude:  points[index].Latitude,
					Longitude: points[index].Longitude,
				}
			}
			found = true
		}
	}
	if !found {
		return &httpError{Status: http.StatusNotFound, Code: 4041, Message: "point not found"}
	}
	payload["path"] = points
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode updated order path: %w", err)
	}
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE orders SET payload=$1,updated_at=NOW()
		WHERE id=$2 AND environment=$3 AND client_id=$4
	`, encoded, orderID, requestEnvironment(r.Context()), clientID(r.Context())); err != nil {
		return err
	}
	orderStatus, _ := payload["status"].(string)
	if err := a.enqueueWebhookTx(tx,
		r.Context(),
		resourceHref(r, orderID),
		map[string]any{"order_id": orderID, "point_id": pointID, "point_status": status},
		"point_status_changed",
		orderStatus,
		clientID(r.Context()),
	); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	a.publishOrder(requestEnvironment(r.Context()), clientID(r.Context()), orderID)
	return nil
}

func validOrderStatus(value string) bool {
	return value == "WAITING_FOR_PAYMENT" || value == "SCHEDULING" || value == "FINDING" ||
		value == "ASSIGNED" ||
		value == "IN PROCESS" ||
		value == "COMPLETED" ||
		value == "CANCELLED"
}

func validPointStatus(value string) bool {
	return value == "CREATED" || value == "ARRIVING" || value == "NEARED" ||
		value == "NEARED_50M" ||
		value == "ARRIVED" ||
		value == "COMPLETED" ||
		value == "FAILED"
}

func resourceHref(r *http.Request, orderID string) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	prefix := "/v1/ride/orders/"
	if strings.HasPrefix(r.URL.Path, "/v1/sandbox/") {
		prefix = "/v1/sandbox/ride/orders/"
	}
	return scheme + "://" + r.Host + prefix + url.PathEscape(orderID)
}

type webhookExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (a *API) enqueueWebhookTx(
	db webhookExecutor, ctx context.Context, resource string, data map[string]any,
	eventType, status, userID string,
) error {
	if a.Config.WebhookURL == "" {
		return nil
	}
	eventID, err := newUUID()
	if err != nil {
		return err
	}
	meta := map[string]any{"resource_id": data["order_id"]}
	if userID != "" {
		meta["user_id"] = userID
	}
	if status != "" {
		meta["status"] = status
	}
	payload := map[string]any{
		"event_id":      eventID,
		"event_time":    time.Now().Unix(),
		"event_type":    eventType,
		"resource_href": resource,
		"data":          data,
		"meta":          meta,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(
		ctx,
		`INSERT INTO webhook_events(event_id,payload) VALUES($1,$2)`,
		eventID,
		encoded,
	)
	return err
}

func webhookSignature(payload []byte, signingKey string) string {
	hasher := hmac.New(sha256.New, []byte(signingKey))
	_, _ = hasher.Write(payload)
	return hex.EncodeToString(hasher.Sum(nil))
}

func (a *API) RunWebhookWorker(ctx context.Context) {
	if a.Config.WebhookURL == "" {
		return
	}
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

func (a *API) deliverOneWebhook(ctx context.Context) error {
	if a.Config.WebhookURL == "" {
		return nil
	}
	var eventID string
	var payload []byte
	err := a.Store.DB.QueryRowContext(ctx, `
		SELECT event_id,payload FROM webhook_events
		WHERE delivered_at IS NULL AND attempts < 7 AND next_attempt_at <= NOW()
		ORDER BY next_attempt_at LIMIT 1
	`).
		Scan(&eventID, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		a.Config.WebhookURL,
		strings.NewReader(string(payload)),
	)
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	var event struct {
		ResourceHref string `json:"resource_href"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode event environment: %w", err)
	}
	environment := "production"
	if strings.Contains(event.ResourceHref, "/v1/sandbox/") {
		environment = "sandbox"
	}
	request.Header.Set("X-Environment", environment)
	if a.Config.WebhookSignKey != "" {
		request.Header.Set("X-VSF-Signature", webhookSignature(payload, a.Config.WebhookSignKey))
	}
	if a.Config.WebhookSignKeySecondary != "" {
		request.Header.Set(
			"X-VSF-Signature-Secondary",
			webhookSignature(payload, a.Config.WebhookSignKeySecondary),
		)
	}
	authorization, authErr := a.webhookAuthorization(ctx)
	if authErr == nil && authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	var response *http.Response
	sendErr := authErr
	if sendErr == nil {
		response, sendErr = a.HTTP.Do(request)
	}
	if sendErr == nil {
		response.Body.Close()
		if response.StatusCode == http.StatusOK {
			_, err = a.Store.DB.ExecContext(
				ctx,
				`UPDATE webhook_events SET delivered_at=NOW(),attempts=attempts+1 WHERE event_id=$1`,
				eventID,
			)
			return err
		}
		sendErr = fmt.Errorf("webhook returned HTTP %d", response.StatusCode)
	}
	var attempts int
	if err := a.Store.DB.QueryRowContext(
		ctx,
		`UPDATE webhook_events SET attempts=attempts+1 WHERE event_id=$1 RETURNING attempts`,
		eventID,
	).Scan(&attempts); err != nil {
		return err
	}
	if attempts < 7 {
		delay := time.Duration(30*(1<<uint(attempts-1))) * time.Second
		_, err = a.Store.DB.ExecContext(
			ctx,
			`UPDATE webhook_events SET next_attempt_at=NOW()+$1::interval WHERE event_id=$2`,
			delay.String(),
			eventID,
		)
		if err != nil {
			return err
		}
	}
	return sendErr
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
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {a.Config.WebhookClientID},
		"client_secret": {a.Config.WebhookClientSecret},
	}
	if a.Config.WebhookScope != "" {
		form.Set("scope", a.Config.WebhookScope)
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		a.Config.WebhookTokenURL,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", fmt.Errorf("create webhook token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := a.HTTP.Do(request)
	if err != nil {
		return "", fmt.Errorf("request webhook access token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("webhook token endpoint returned HTTP %d", response.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return "", fmt.Errorf("decode webhook token response: %w", err)
	}
	if token.AccessToken == "" || !strings.EqualFold(token.TokenType, "Bearer") ||
		token.ExpiresIn <= 0 {
		return "", fmt.Errorf(
			"webhook token response requires access_token, Bearer token_type, and positive expires_in",
		)
	}
	a.webhookToken = token.AccessToken
	a.webhookTokenExpires = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	return "Bearer " + token.AccessToken, nil
}
