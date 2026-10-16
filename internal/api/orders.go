package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (a *API) createOrder(w http.ResponseWriter, r *http.Request) {
	var input CreateOrderInput
	if !a.readJSON(w, r, &input) {
		return
	}
	if input.ServiceID <= 0 || input.FeeID == "" || input.ServiceType == "" ||
		!validPayment(input.PaymentMethod) {
		writeError(
			w,
			http.StatusBadRequest,
			400,
			"service_id, service_type, fee_id, and valid payment_method are required",
		)
		return
	}
	if len(input.PartnerTripID) > 32 {
		writeError(w, http.StatusBadRequest, 400, "partner_trip_id must be at most 32 characters")
		return
	}
	requireDropoff := input.ServiceType != "RIDE-HOUR"
	if err := validatePoints(input.Points, requireDropoff, true); err != nil {
		writeError(w, http.StatusBadRequest, 400, err.Error())
		return
	}
	if input.ServiceType == "RIDE-TRIP" && input.PartnerPaymentMethod == "B2B" {
		if err := validateGreenConnect(input); err != nil {
			writeError(w, http.StatusBadRequest, 400, err.Error())
			return
		}
	}
	if input.Addons == nil {
		input.Addons = []Addon{}
	}
	if _, err := validateAddons(input.Addons, false); err != nil {
		writeError(w, http.StatusBadRequest, 400, err.Error())
		return
	}
	quote, err := a.loadQuote(r, input.FeeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusBadRequest, 400, "fee_id is invalid or expired")
			return
		}
		a.dbError(w, "load estimate", err)
		return
	}
	if quote.ClientID != clientID(r.Context()) ||
		quote.Environment != requestEnvironment(r.Context()) ||
		quote.ExpiresAt < time.Now().Unix() {
		writeError(w, http.StatusBadRequest, 400, "fee_id is invalid or expired")
		return
	}
	if quote.ServiceID != input.ServiceID || quote.ServiceType != input.ServiceType ||
		quote.OrderTime != input.OrderTime ||
		quote.PaymentMethod != input.PaymentMethod ||
		!jsonEqual(quote.Addons, input.Addons) ||
		!samePoints(quote.Points, input.Points) {
		writeError(
			w,
			http.StatusBadRequest,
			400,
			"booking fields must match the estimate used for fee_id",
		)
		return
	}
	if input.OrderTime == 0 {
		input.OrderTime = time.Now().Unix()
	}
	var travelMode string
	err = a.Store.DB.QueryRowContext(r.Context(), `
		SELECT travel_mode FROM services WHERE id=$1
	`, input.ServiceID).
		Scan(&travelMode)
	if err != nil {
		a.dbError(w, "load service for order", err)
		return
	}
	orderID, err := newUUID()
	if err != nil {
		a.dbError(w, "generate order id", err)
		return
	}
	payload, err := a.orderPayload(orderID, clientID(r.Context()), input, travelMode)
	if err != nil {
		a.dbError(w, "build order response", err)
		return
	}
	payload["fare"] = quote.Response
	encoded, _ := json.Marshal(payload)
	tx, err := a.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		a.dbError(w, "begin order", err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(r.Context(), `
		INSERT INTO orders(
			id,environment,client_id,status,order_time,payment_method,travel_mode,
			service_type,service_id,partner_trip_id,payload
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, orderID, requestEnvironment(r.Context()), clientID(r.Context()),
		"FINDING", input.OrderTime, input.PaymentMethod, travelMode,
		input.ServiceType, input.ServiceID, input.PartnerTripID, encoded,
	); err != nil {
		a.dbError(w, "save order", err)
		return
	}
	resource := resourceHref(r, orderID)
	if err := a.enqueueWebhookTx(
		tx, r.Context(), resource,
		map[string]any{"order_id": orderID, "status": "FINDING"},
		"order_status_changed", "FINDING", clientID(r.Context()),
	); err != nil {
		a.dbError(w, "queue initial order webhook", err)
		return
	}
	if requestEnvironment(r.Context()) == "sandbox" {
		_, err = tx.ExecContext(r.Context(), `
			INSERT INTO sandbox_runs(order_id,resource_href,next_step_at)
			VALUES($1,$2,GREATEST(NOW(),to_timestamp($3))+INTERVAL '5 seconds')
		`, orderID, resource, input.OrderTime)
		if err != nil {
			a.dbError(w, "schedule sandbox trip", err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		a.dbError(w, "commit order", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"id": orderID, "partner_trip_id": input.PartnerTripID, "status": "FINDING",
		"order_time": fmt.Sprint(input.OrderTime), "path": payload["path"], "geofence_id": "",
	}})
}

func (a *API) orderPayload(
	orderID, ownerID string,
	input CreateOrderInput,
	travelMode string,
) (map[string]any, error) {
	now := time.Now().Unix()
	path := make([]Point, 0, len(input.Points))
	for _, point := range input.Points {
		pointID, err := newUUID()
		if err != nil {
			return nil, err
		}
		point.ID, point.Status = pointID, "CREATED"
		path = append(path, point)
	}
	var displayName string
	if err := a.Store.DB.QueryRow(
		`SELECT display_name FROM services WHERE id=$1`, input.ServiceID,
	).Scan(&displayName); err != nil {
		return nil, fmt.Errorf("load service display name: %w", err)
	}
	service := map[string]any{
		"display_name":         displayName,
		"service_id":           input.ServiceID,
		"service_type":         input.ServiceType,
		"addons":               input.Addons,
		"addon_groups":         []any{},
		"partner_service_type": "",
	}
	return map[string]any{
		"id": orderID, "partner_trip_id": input.PartnerTripID, "status": "FINDING",
		"order_time": fmt.Sprint(input.OrderTime), "path": path,
		"payment_method": input.PaymentMethod, "vehicle": map[string]any{},
		"driver": map[string]any{}, "customer": map[string]any{}, "service": service,
		"tracking_url": "", "created_by": ownerID, "cancel_code": map[string]any{},
		"create_time": fmt.Sprint(now), "dispatch_time": fmt.Sprint(now),
		"travel_mode": travelMode, "expiring_time": fmt.Sprint(now + 300),
		"driver_lock_time": "0", "service_type": input.ServiceType,
		"vehicle_models": []string{}, "pricing_mode": pricingMode(input.ServiceType),
		"pay_type": input.PaymentMethod, "partner_id": "local-partner",
		"platform": "partner", "current_server_time": fmt.Sprint(now),
		"application_id": ownerID, "schedule_info": map[string]any{},
	}, nil
}

func pricingMode(serviceType string) string {
	if serviceType == "RIDE-HOUR" {
		return "HOURLY_DEMO"
	}
	return "HANOI_REFERENCE_SIMULATION"
}

func (a *API) getOrder(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	err := a.Store.DB.QueryRowContext(r.Context(), `
		SELECT payload FROM orders
		WHERE id=$1 AND environment=$2 AND client_id=$3
	`, r.PathValue("id"), requestEnvironment(r.Context()), clientID(r.Context())).
		Scan(jsonScanner{target: &payload})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, 4031, "order not found")
			return
		}
		a.dbError(w, "load order details", err)
		return
	}
	components := strings.Split(r.URL.Query().Get("extra_components"), ",")
	if !contains(components, "path") {
		delete(payload, "path")
	}
	if !contains(components, "driver") {
		delete(payload, "driver")
		delete(payload, "vehicle")
	}
	payload["current_server_time"] = fmt.Sprint(time.Now().Unix())
	writeJSON(w, http.StatusOK, map[string]any{"data": payload})
}

func (a *API) listOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := a.Store.DB.QueryContext(
		r.Context(),
		`SELECT id,payload FROM orders WHERE environment=$1 AND client_id=$2 ORDER BY created_at DESC`,
		requestEnvironment(r.Context()),
		clientID(r.Context()),
	)
	if err != nil {
		a.dbError(w, "list orders", err)
		return
	}
	defer rows.Close()
	data := make([]map[string]any, 0)
	for rows.Next() {
		var id string
		var payload map[string]any
		if err := rows.Scan(&id, jsonScanner{target: &payload}); err != nil {
			a.dbError(w, "read order", err)
			return
		}
		path, _ := payload["path"]
		data = append(data, map[string]any{
			"order_id": id, "service": payload["service"], "status": payload["status"],
			"order_time": payload["order_time"], "path": path,
			"driver_lock_time": payload["driver_lock_time"],
			"payment_method":   payload["payment_method"], "travel_mode": payload["travel_mode"],
		})
	}
	if err := rows.Err(); err != nil {
		a.dbError(w, "iterate order list", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

func (a *API) cancelOrder(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OrderID       string `json:"order_id"`
		CancelCode    int    `json:"cancel_code"`
		CancelMessage string `json:"cancel_message"`
	}
	if !a.readJSON(w, r, &input) {
		return
	}
	if input.OrderID == "" || input.CancelCode <= 0 ||
		strings.TrimSpace(input.CancelMessage) == "" {
		writeError(
			w,
			http.StatusBadRequest,
			4017,
			"order_id, cancel_code, and cancel_message are required",
		)
		return
	}
	if err := a.changeOrderStatus(
		r, input.OrderID, "CANCELLED",
		map[string]any{"code": input.CancelCode, "message": input.CancelMessage},
	); err != nil {
		var apiErr *httpError
		if errors.As(err, &apiErr) {
			writeError(w, apiErr.Status, apiErr.Code, apiErr.Message)
			return
		}
		a.dbError(w, "cancel order", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"message": "ok"}})
}

func samePoints(left, right []Point) bool {
	for index := range left {
		if index >= len(right) || left[index].Type != right[index].Type ||
			left[index].Latitude != right[index].Latitude ||
			left[index].Longitude != right[index].Longitude ||
			left[index].Name != right[index].Name ||
			left[index].PhoneNumber != right[index].PhoneNumber ||
			left[index].Address != right[index].Address ||
			left[index].ShortAddress != right[index].ShortAddress {
			return false
		}
	}
	return len(left) == len(right)
}

type httpError struct {
	Status  int
	Code    int
	Message string
}

func (e *httpError) Error() string { return e.Message }
