package api

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

func validateBookingRequest(input CreateOrderInput) error {
	if input.ServiceID <= 0 || input.FeeID == "" || input.ServiceType == "" ||
		!validPayment(input.PaymentMethod) {
		return fmt.Errorf("service_id, service_type, fee_id, and valid payment_method are required")
	}
	if len(input.PartnerTripID) > 32 {
		return fmt.Errorf("partner_trip_id must be at most 32 characters")
	}
	if err := validatePoints(input.Points, input.ServiceType != "RIDE-HOUR", true); err != nil {
		return err
	}
	return validateBookingOptions(input)
}

func validateBookingOptions(input CreateOrderInput) error {
	if _, err := validateAddons(input.Addons, false); err != nil {
		return err
	}
	if input.ServiceType == "RIDE-TRIP" && input.PartnerPaymentMethod == "B2B" {
		return validateGreenConnect(input)
	}
	return nil
}

func validateQuote(r *http.Request, input CreateOrderInput, quote quote) error {
	if quote.ClientID != clientID(r.Context()) ||
		quote.Environment != requestEnvironment(r.Context()) {
		return &httpError{Status: 400, Code: 400, Message: "fee_id is invalid or expired"}
	}
	if !quoteMatches(input, quote) {
		return &httpError{Status: 400, Code: 400,
			Message: "booking fields must match the estimate used for fee_id"}
	}
	return nil
}

func quoteMatches(input CreateOrderInput, quote quote) bool {
	return quote.ServiceID == input.ServiceID && quote.ServiceType == input.ServiceType &&
		quote.OrderTime == input.OrderTime && quote.PaymentMethod == input.PaymentMethod &&
		jsonEqual(quote.Addons, input.Addons) && samePoints(quote.Points, input.Points)
}

func (a *API) createQuotedOrder(r *http.Request, input CreateOrderInput,
	quote quote,
) (map[string]any, error) {
	if err := validateQuote(r, input, quote); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	bookingHash := fmt.Sprintf("%x", sha256.Sum256(encoded))
	tx, err := a.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var feeID string
	if err := tx.QueryRowContext(r.Context(),
		`SELECT fee_id FROM estimates WHERE fee_id=$1 FOR UPDATE`, quote.FeeID).Scan(&feeID); err != nil {
		return nil, err
	}
	payload, err := replayBooking(r, tx, quote.FeeID, bookingHash)
	if err != nil || payload != nil {
		return payload, err
	}
	return a.acceptBooking(r, tx, input, quote, bookingHash)
}

func (a *API) acceptBooking(r *http.Request, tx *sql.Tx, input CreateOrderInput,
	quote quote, bookingHash string,
) (map[string]any, error) {
	if quote.ExpiresAt <= time.Now().Unix() {
		return nil, &httpError{Status: 400, Code: 400, Message: "fee_id is invalid or expired"}
	}
	payload, err := a.newBookingPayload(r, tx, input)
	if err != nil {
		return nil, err
	}
	payload["fare"] = quote.Response
	if err := a.persistBooking(r, tx, payload, input, bookingHash); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return payload, nil
}

func replayBooking(r *http.Request, tx *sql.Tx, feeID, bookingHash string,
) (map[string]any, error) {
	var payload map[string]any
	var savedHash string
	err := tx.QueryRowContext(r.Context(),
		`SELECT payload,booking_hash FROM orders WHERE quote_fee_id=$1`, feeID).
		Scan(jsonScanner{target: &payload}, &savedHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if bookingHash != savedHash {
		return nil, &httpError{Status: 409, Code: 4013,
			Message: "fee_id already booked with different fields; obtain a new estimate"}
	}
	return payload, nil
}

func (a *API) newBookingPayload(r *http.Request, tx *sql.Tx,
	input CreateOrderInput,
) (map[string]any, error) {
	if err := validateInbox(r, tx, input.WebhookInboxID); err != nil {
		return nil, err
	}
	var mode string
	err := tx.QueryRowContext(r.Context(),
		`SELECT travel_mode FROM services WHERE id=$1`, input.ServiceID).Scan(&mode)
	if err != nil {
		return nil, err
	}
	id, err := newUUID()
	if err != nil {
		return nil, err
	}
	if input.OrderTime == 0 {
		input.OrderTime = time.Now().Unix()
	}
	payload, err := a.orderPayload(id, clientID(r.Context()), input, mode)
	if err != nil {
		return nil, err
	}
	if input.OrderTime > time.Now().Unix() {
		payload["status"] = "SCHEDULING"
	}
	if requestEnvironment(r.Context()) == "sandbox" {
		initializeSimulation(payload, input)
	}
	payload["webhook_inbox_id"] = input.WebhookInboxID
	return payload, nil
}

func initializeSimulation(payload map[string]any, input CreateOrderInput) {
	scenario := input.SandboxScenario
	if scenario == "" {
		scenario = "normal"
	}
	payload["simulation"] = simulationState{
		Scenario: scenario, ExcludedDrivers: []string{}, Event: "created",
	}
	if input.PaymentMethod == "partner_pay" {
		payload["status"] = "WAITING_FOR_PAYMENT"
	}
}

func (a *API) persistBooking(r *http.Request, tx *sql.Tx,
	payload map[string]any, input CreateOrderInput, bookingHash string,
) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	environment := requestEnvironment(r.Context())
	_, err = tx.ExecContext(r.Context(), `INSERT INTO orders(
		id,environment,client_id,status,order_time,payment_method,travel_mode,
		service_type,service_id,partner_trip_id,payload,quote_fee_id,booking_hash)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		payload["id"], environment, clientID(r.Context()), payload["status"],
		payload["order_time"], input.PaymentMethod, payload["travel_mode"],
		input.ServiceType, input.ServiceID, input.PartnerTripID, encoded, input.FeeID, bookingHash)
	if err != nil {
		return err
	}
	id, status := payload["id"].(string), payload["status"].(string)
	resource := resourceHref(r, id)
	if err := a.enqueueWebhookTx(tx, r.Context(), resource,
		map[string]any{"order_id": id, "status": status},
		"order_status_changed", status, clientID(r.Context())); err != nil {
		return err
	}
	if environment != "sandbox" {
		return nil
	}
	_, err = tx.ExecContext(r.Context(), `
		INSERT INTO sandbox_runs(order_id,resource_href,next_step_at)
		VALUES($1,$2,GREATEST(NOW(),to_timestamp($3))+INTERVAL '5 seconds')`,
		id, resource, input.OrderTime)
	return err
}
