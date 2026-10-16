package api

import (
	"database/sql"
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
	if input.Addons == nil {
		input.Addons = []Addon{}
	}
	if err := validateBookingRequest(input); err != nil {
		writeError(w, http.StatusBadRequest, 400, err.Error())
		return
	}
	if err := validScenario(input, requestEnvironment(r.Context())); err != nil {
		a.writeOperationError(w, "validate simulation", err)
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
	payload, err := a.createQuotedOrder(r, input, quote)
	if err != nil {
		a.writeOperationError(w, "create order", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": payload})
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
		"geofence_id": "",
		"order_time":  fmt.Sprint(input.OrderTime), "path": path,
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
