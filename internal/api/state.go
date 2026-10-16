package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type statusUpdate struct {
	UpdateType  string `json:"update_type"`
	OrderID     string `json:"order_id"`
	Status      string `json:"status"`
	PointStatus string `json:"point_status"`
	PointID     string `json:"point_id"`
}

func (a *API) updateStatus(w http.ResponseWriter, r *http.Request) {
	var input statusUpdate
	if !a.readJSON(w, r, &input) {
		return
	}
	if input.OrderID == "" {
		writeError(w, http.StatusBadRequest, 400, "order_id is required")
		return
	}
	if err := a.applyStatusUpdate(r, input); err != nil {
		a.writeOperationError(w, "update simulated status", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"message": "ok"}})
}

func (a *API) applyStatusUpdate(r *http.Request, input statusUpdate) error {
	switch input.UpdateType {
	case "order":
		if !validOrderStatus(input.Status) {
			return &httpError{Status: 400, Code: 400, Message: "status is not a supported order status"}
		}
		return a.changeOrderStatus(r, input.OrderID, input.Status, nil)
	case "point":
		if input.PointID == "" || !validPointStatus(input.PointStatus) {
			return &httpError{Status: 400, Code: 400,
				Message: "point_id and a supported point_status are required"}
		}
		return a.changePointStatus(r, input.OrderID, input.PointID, input.PointStatus)
	default:
		return &httpError{Status: 400, Code: 400, Message: "update_type must be order or point"}
	}
}

func loadLockedOrder(r *http.Request, tx *sql.Tx, id string) (map[string]any, error) {
	var payload map[string]any
	var status string
	err := tx.QueryRowContext(r.Context(), `SELECT status,payload FROM orders
  WHERE id=$1 AND environment=$2 AND client_id=$3 FOR UPDATE`,
		id, requestEnvironment(r.Context()), clientID(r.Context())).
		Scan(&status, jsonScanner{target: &payload})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &httpError{Status: 404, Code: 4031, Message: "order not found"}
	}
	if err != nil {
		return nil, err
	}
	payload["status"] = status
	return payload, nil
}

func (a *API) changeOrderStatus(r *http.Request, orderID, status string, cancelCode any) error {
	tx, err := a.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	payload, err := loadLockedOrder(r, tx, orderID)
	if err != nil {
		return err
	}
	changed, err := a.prepareOrderStatus(r, tx, payload, status)
	if err != nil || !changed {
		return err
	}
	if err := a.persistOrderStatus(r, tx, payload, cancelCode); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	a.publishOrder(requestEnvironment(r.Context()), clientID(r.Context()), orderID)
	return nil
}

func (a *API) prepareOrderStatus(r *http.Request, tx *sql.Tx,
	payload map[string]any, status string,
) (bool, error) {
	oldStatus, _ := payload["status"].(string)
	if oldStatus == status {
		return false, nil
	}
	if !canTransitionOrder(oldStatus, status) {
		return false, &httpError{Status: 409, Code: 4013,
			Message: fmt.Sprintf("cannot change order from %s to %s", oldStatus, status)}
	}
	if status == "ASSIGNED" {
		driver, err := a.reserveDriver(r.Context(), tx, payload, requestEnvironment(r.Context()), nil)
		if err != nil {
			return false, err
		}
		if driver == nil {
			return false, &httpError{Status: 409, Code: 4013,
				Message: "no available vehicle within 10 km; retry later"}
		}
		assignDriver(payload, *driver)
	}
	if payload["simulation"] != nil {
		simulation, err := readSimulation(payload)
		if err != nil {
			return false, err
		}
		simulation.Event = status
		payload["simulation"] = simulation
	}
	payload["status"], payload["current_server_time"] = status, fmt.Sprint(time.Now().Unix())
	return true, nil
}

func (a *API) persistOrderStatus(r *http.Request, tx *sql.Tx,
	payload map[string]any, cancelCode any,
) error {
	if cancelCode != nil {
		payload["cancel_code"] = cancelCode
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode updated order: %w", err)
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE orders SET status=$1,payload=$2,
  driver_lock_time=($2::jsonb->>'driver_lock_time')::bigint,
  driver_id=(SELECT id FROM simulated_drivers WHERE id=($2::jsonb->'driver'->>'id')),
  updated_at=NOW() WHERE id=$3 AND environment=$4 AND client_id=$5`,
		payload["status"], encoded, payload["id"],
		requestEnvironment(r.Context()), clientID(r.Context()))
	if err != nil {
		return err
	}
	status := payload["status"].(string)
	return a.enqueueWebhookTx(tx, r.Context(), resourceHref(r, payload["id"].(string)),
		map[string]any{"order_id": payload["id"], "status": status},
		"order_status_changed", status, clientID(r.Context()))
}

func (a *API) changePointStatus(r *http.Request, orderID, pointID, status string) error {
	tx, err := a.Store.DB.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	payload, err := loadLockedOrder(r, tx, orderID)
	if err != nil {
		return err
	}
	changed, err := changePathPoint(payload, pointID, status)
	if err != nil || !changed {
		return err
	}
	if err := a.persistPointStatus(r, tx, payload, pointID, status); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	a.publishOrder(requestEnvironment(r.Context()), clientID(r.Context()), orderID)
	return nil
}

func changePathPoint(payload map[string]any, pointID, status string) (bool, error) {
	if payload["status"] != "ASSIGNED" && payload["status"] != "IN PROCESS" {
		return false, &httpError{Status: 409, Code: 4013,
			Message: "point changes require an assigned or in-process order"}
	}
	points, err := orderPoints(payload)
	if err != nil {
		return false, err
	}
	point := findOrderPoint(points, pointID)
	if point == nil {
		return false, &httpError{Status: 404, Code: 4041, Message: "point not found"}
	}
	if point.Status == status {
		return false, nil
	}
	if !canTransitionPoint(point.Status, status) {
		return false, &httpError{Status: 409, Code: 4013,
			Message: fmt.Sprintf("cannot change point from %s to %s", point.Status, status)}
	}
	applyPointStatus(point, status)
	payload["path"] = points
	payload["current_server_time"] = fmt.Sprint(time.Now().Unix())
	return true, nil
}

func findOrderPoint(points []Point, id string) *Point {
	for index := range points {
		if points[index].ID == id {
			return &points[index]
		}
	}
	return nil
}

func (a *API) persistPointStatus(r *http.Request, tx *sql.Tx,
	payload map[string]any, pointID, status string,
) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode updated order path: %w", err)
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE orders SET payload=$1,updated_at=NOW()
  WHERE id=$2 AND environment=$3 AND client_id=$4`, encoded, payload["id"],
		requestEnvironment(r.Context()), clientID(r.Context()))
	if err != nil {
		return err
	}
	return a.enqueueWebhookTx(tx, r.Context(), resourceHref(r, payload["id"].(string)),
		map[string]any{"order_id": payload["id"], "point_id": pointID, "point_status": status},
		"point_status_changed", payload["status"].(string), clientID(r.Context()))
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
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (a *API) enqueueWebhookTx(
	db webhookExecutor, ctx context.Context, resource string, data map[string]any,
	eventType, status, userID string,
) error {
	target, order, err := a.webhookDestination(db, ctx, data["order_id"], userID)
	if err != nil {
		return err
	}
	if target == "" {
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
	data = webhookEventData(data, order)
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
		`INSERT INTO webhook_events(event_id,payload,target_url) VALUES($1,$2,$3)`,
		eventID,
		encoded,
		target,
	)
	return err
}
