package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gsm-api/internal/store"
)

type Config struct {
	LocalOrigin             string
	ClientID                string
	ClientSecret            string
	NominatimURL            string
	OSRMURL                 string
	WebhookURL              string
	WebhookSignKey          string
	WebhookSignKeySecondary string
	WebhookAuth             string
	WebhookTokenURL         string
	WebhookClientID         string
	WebhookClientSecret     string
	WebhookScope            string
}

type API struct {
	orderHub            orderHub
	Store               *store.Store
	Config              Config
	HTTP                *http.Client
	Logger              *log.Logger
	OSM                 *OSM
	webhookTokenMu      sync.Mutex
	webhookToken        string
	webhookTokenExpires time.Time
}

func New(db *store.Store, config Config, logger *log.Logger) *API {
	api := &API{
		Store: db, Config: config,
		HTTP:   &http.Client{Timeout: 8 * time.Second},
		Logger: logger,
	}
	api.OSM = NewOSM(config.NominatimURL, config.OSRMURL, api.HTTP)
	return api
}

func ConfigFromEnv() Config {
	return Config{
		ClientID:                envOr("CLIENT_ID", "local-client"),
		ClientSecret:            envOr("CLIENT_SECRET", "local-secret"),
		NominatimURL:            envOr("NOMINATIM_URL", "https://nominatim.openstreetmap.org"),
		OSRMURL:                 envOr("OSRM_URL", "https://router.project-osrm.org"),
		WebhookURL:              os.Getenv("WEBHOOK_URL"),
		WebhookSignKey:          os.Getenv("WEBHOOK_SIGNING_KEY"),
		WebhookSignKeySecondary: os.Getenv("WEBHOOK_SIGNING_KEY_SECONDARY"),
		WebhookAuth:             os.Getenv("WEBHOOK_AUTHORIZATION"),
		WebhookTokenURL:         os.Getenv("WEBHOOK_TOKEN_URL"),
		WebhookClientID:         os.Getenv("WEBHOOK_CLIENT_ID"),
		WebhookClientSecret:     os.Getenv("WEBHOOK_CLIENT_SECRET"),
		WebhookScope:            os.Getenv("WEBHOOK_SCOPE"),
	}
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/sandbox/token", a.handleToken())
	mux.Handle("POST /v1/sandbox/ride/list-available-services", a.authorized(a.listServices))
	mux.Handle("POST /v1/sandbox/ride/estimate/order", a.authorized(a.estimateTrip))
	mux.Handle("POST /v1/sandbox/ride/estimate/hour-order", a.authorized(a.estimateHour))
	mux.Handle("POST /v1/sandbox/ride/create-order", a.authorized(a.createOrder))
	mux.Handle("GET /v1/sandbox/ride/orders/{id}", a.authorized(a.getOrder))
	mux.Handle("GET /v1/sandbox/ride/orders/{id}/events", a.authorized(a.streamOrder))
	mux.Handle("GET /v1/sandbox/ride/list-user-orders", a.authorized(a.listOrders))
	mux.Handle("POST /v1/sandbox/ride/cancel-order", a.authorized(a.cancelOrder))
	mux.Handle("GET /v1/sandbox/place/geocode", a.authorized(a.geocode))
	mux.Handle("POST /v1/sandbox/ride/update-status", a.authorized(a.updateStatus))
	mux.Handle("POST /v1/sandbox/webhook-inboxes", a.authorized(a.createWebhookInbox))
	mux.Handle("GET /v1/sandbox/webhook-inboxes/{id}", a.authorized(a.getWebhookInbox))
	mux.Handle("GET /v1/sandbox/webhook-inboxes/{id}/receipts/{receipt}",
		a.authorized(a.getWebhookReceipt))
	mux.HandleFunc("POST /webhook-inbox/{token}", a.receiveWebhook)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return recoverMiddleware(a.Logger, mux)
}

func (a *API) handleToken() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			writeError(w, http.StatusBadRequest, 400, "invalid form body")
			return
		}
		const scope = "sandbox.ride.trips"
		if r.Form.Get("client_id") != a.Config.ClientID ||
			r.Form.Get("client_secret") != a.Config.ClientSecret {
			writeError(w, http.StatusUnauthorized, 401, "invalid client credentials")
			return
		}
		if r.Form.Get("grant_type") != "client_credentials" {
			writeError(w, http.StatusBadRequest, 400, "grant_type must be client_credentials")
			return
		}
		if r.Form.Get("scope") != "" && r.Form.Get("scope") != scope {
			writeError(w, http.StatusBadRequest, 400, "invalid scope for sandbox token")
			return
		}

		token, err := randomHex(32)
		if err != nil {
			writeError(w, http.StatusInternalServerError, 500, "could not generate access token")
			return
		}
		expires := time.Now().Add(30 * 24 * time.Hour).Unix()
		_, err = a.Store.DB.ExecContext(
			r.Context(),
			`INSERT INTO access_tokens(token_hash,client_id,scope,expires_at) VALUES($1,$2,$3,$4)`,
			hashToken(token),
			a.Config.ClientID,
			scope,
			expires,
		)
		if err != nil {
			a.dbError(w, "store access token", err)
			return
		}
		writeJSON(
			w,
			http.StatusOK,
			map[string]any{
				"access_token": token,
				"token_type":   "Bearer",
				"expires_in":   2592000,
				"scope":        scope,
			},
		)
	}
}

type requestContextKey string

const clientContextKey requestContextKey = "client_id"
const environmentContextKey requestContextKey = "environment"

func (a *API) authorized(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			writeError(w, http.StatusUnauthorized, 401, "missing or invalid bearer token")
			return
		}
		var clientID, scope string
		var expires int64
		err := a.Store.DB.QueryRowContext(
			r.Context(),
			`SELECT client_id,scope,expires_at FROM access_tokens WHERE token_hash=$1`,
			hashToken(parts[1]),
		).
			Scan(&clientID, &scope, &expires)
		const want = "sandbox.ride.trips"
		if errors.Is(err, sql.ErrNoRows) ||
			(err == nil && (expires < time.Now().Unix() || scope != want)) {
			writeError(
				w,
				http.StatusUnauthorized,
				401,
				"token is invalid, expired, or missing the required scope",
			)
			return
		}
		if err != nil {
			a.dbError(w, "validate access token", err)
			return
		}
		const environment = "sandbox"
		ctx := context.WithValue(r.Context(), clientContextKey, clientID)
		ctx = context.WithValue(ctx, environmentContextKey, environment)
		next(w, r.WithContext(ctx))
	})
}

func clientID(ctx context.Context) string {
	value, _ := ctx.Value(clientContextKey).(string)
	return value
}

func requestEnvironment(ctx context.Context) string {
	value, _ := ctx.Value(environmentContextKey).(string)
	return value
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func randomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("read secure random bytes: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func decodeJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 2<<20))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode JSON request: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status, code int, message string) {
	writeJSON(w, status, map[string]any{"code": code, "message": message})
}

func (a *API) dbError(w http.ResponseWriter, operation string, err error) {
	a.Logger.Printf("%s: %v", operation, err)
	writeError(w, http.StatusInternalServerError, 500, "database operation failed")
}

func (a *API) readJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	if err := decodeJSON(r, value); err != nil {
		writeError(w, http.StatusBadRequest, 400, "invalid JSON request body")
		return false
	}
	return true
}

func recoverMiddleware(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				logger.Printf("panic serving %s: %v", r.URL.Path, value)
				writeError(w, http.StatusInternalServerError, 500, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func envOr(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
