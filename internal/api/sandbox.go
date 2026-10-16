package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RunSandboxWorker advances persisted sandbox trips until the context is cancelled.
func (a *API) RunSandboxWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for range 100 {
				advanced, err := a.advanceSandbox(ctx)
				if err != nil {
					a.Logger.Printf("advance sandbox trip: %v", err)
					break
				}
				if !advanced {
					break
				}
			}
		}
	}
}

func (a *API) advanceSandbox(ctx context.Context) (bool, error) {
	tx, err := a.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var id, resource, owner, status string
	var payload map[string]any
	err = tx.QueryRowContext(ctx, `
  SELECT o.id,r.resource_href,o.client_id,o.status,o.payload
  FROM sandbox_runs r JOIN orders o ON o.id=r.order_id
  WHERE r.next_step_at<=NOW() AND o.environment='sandbox'
  ORDER BY r.next_step_at FOR UPDATE OF o,r SKIP LOCKED LIMIT 1
 `).Scan(&id, &resource, &owner, &status, jsonScanner{target: &payload})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if status == "CANCELLED" || status == "COMPLETED" {
		_, err = tx.ExecContext(ctx, `DELETE FROM sandbox_runs WHERE order_id=$1`, id)
		if err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	data, eventType, err := sandboxStep(payload)
	if err != nil {
		return false, fmt.Errorf("sandbox order %s: %w", id, err)
	}
	data["order_id"] = id
	encoded, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `
  UPDATE orders SET status=$1,payload=$2,updated_at=NOW(),
  driver_lock_time=($2::jsonb->>'driver_lock_time')::bigint WHERE id=$3
 `, payload["status"], encoded, id)
	if err != nil {
		return false, err
	}
	if err = a.enqueueWebhookTx(
		tx, ctx, resource, data, eventType, payload["status"].(string), owner,
	); err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `
  UPDATE sandbox_runs SET next_step_at=NOW()+INTERVAL '5 seconds' WHERE order_id=$1
 `, id)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	a.publishOrder("sandbox", owner, id)
	return true, nil
}

func sandboxStep(payload map[string]any) (map[string]any, string, error) {
	status, _ := payload["status"].(string)
	now := fmt.Sprint(time.Now().Unix())
	payload["current_server_time"] = now
	if status == "FINDING" || status == "SCHEDULING" || status == "WAITING_FOR_PAYMENT" {
		payload["status"] = "ASSIGNED"
		assignSimulatedDriver(payload)
		return map[string]any{"status": "ASSIGNED"}, "order_status_changed", nil
	}
	if status != "ASSIGNED" && status != "IN PROCESS" {
		return nil, "", fmt.Errorf("unsupported active status %q", status)
	}
	encoded, err := json.Marshal(payload["path"])
	if err != nil {
		return nil, "", err
	}
	var points []Point
	if err = json.Unmarshal(encoded, &points); err != nil {
		return nil, "", err
	}
	if len(points) == 0 {
		return nil, "", fmt.Errorf("trip has no path points")
	}
	for index := range points {
		point := &points[index]
		if point.Status == "COMPLETED" || point.Status == "FAILED" {
			continue
		}
		if status == "ASSIGNED" && index > 0 {
			break
		}
		point.Status = nextSandboxPointStatus(point.Status)
		if point.Status == "ARRIVED" {
			point.ArriveTime = now
		}
		if point.Status == "COMPLETED" {
			point.ProofOfWork = &GPS{Latitude: point.Latitude, Longitude: point.Longitude}
		}
		payload["path"] = points
		return map[string]any{"point_id": point.ID, "point_status": point.Status},
			"point_status_changed", nil
	}
	next := "COMPLETED"
	if status == "ASSIGNED" {
		next = "IN PROCESS"
	}
	payload["status"] = next
	return map[string]any{"status": next}, "order_status_changed", nil
}

func nextSandboxPointStatus(status string) string {
	switch status {
	case "CREATED":
		return "ARRIVING"
	case "ARRIVING":
		return "NEARED"
	case "NEARED", "NEARED_50M":
		return "ARRIVED"
	default:
		return "COMPLETED"
	}
}
