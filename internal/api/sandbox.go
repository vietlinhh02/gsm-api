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

type sandboxRun struct {
	ID, Resource, Owner, Status string
	Payload                     map[string]any
}

func (a *API) advanceSandbox(ctx context.Context) (bool, error) {
	tx, err := a.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	run, err := loadSandboxRun(ctx, tx)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := a.advanceLockedSandbox(ctx, tx, run); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	a.publishOrder("sandbox", run.Owner, run.ID)
	return true, nil
}

func loadSandboxRun(ctx context.Context, tx *sql.Tx) (sandboxRun, error) {
	var run sandboxRun
	err := tx.QueryRowContext(ctx, `
  SELECT o.id,r.resource_href,o.client_id,o.status,o.payload
  FROM sandbox_runs r JOIN orders o ON o.id=r.order_id
  WHERE r.next_step_at<=NOW() AND o.environment='sandbox'
  ORDER BY r.next_step_at FOR UPDATE OF o,r SKIP LOCKED LIMIT 1`).
		Scan(&run.ID, &run.Resource, &run.Owner, &run.Status, jsonScanner{target: &run.Payload})
	return run, err
}

func (a *API) advanceLockedSandbox(ctx context.Context, tx *sql.Tx, run sandboxRun) error {
	if run.Status == "CANCELLED" || run.Status == "COMPLETED" {
		_, err := tx.ExecContext(ctx, `DELETE FROM sandbox_runs WHERE order_id=$1`, run.ID)
		return err
	}
	data, eventType, err := a.advanceSimulation(ctx, tx, run.Payload)
	if err != nil {
		return fmt.Errorf("sandbox order %s: %w", run.ID, err)
	}
	data["order_id"] = run.ID
	return a.persistSandboxStep(ctx, tx, run, data, eventType)
}

func (a *API) persistSandboxStep(ctx context.Context, tx *sql.Tx, run sandboxRun,
	data map[string]any, eventType string,
) error {
	encoded, err := json.Marshal(run.Payload)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE orders SET status=$1,payload=$2,updated_at=NOW(),
  driver_lock_time=($2::jsonb->>'driver_lock_time')::bigint,
  driver_id=(SELECT id FROM simulated_drivers WHERE id=($2::jsonb->'driver'->>'id'))
  WHERE id=$3`, run.Payload["status"], encoded, run.ID)
	if err != nil {
		return err
	}
	status := run.Payload["status"].(string)
	if err := a.enqueueWebhookTx(tx, ctx, run.Resource, data, eventType, status, run.Owner); err != nil {
		return err
	}
	if status == "COMPLETED" || status == "CANCELLED" {
		_, err = tx.ExecContext(ctx, `DELETE FROM sandbox_runs WHERE order_id=$1`, run.ID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE sandbox_runs
   SET next_step_at=NOW()+INTERVAL '5 seconds' WHERE order_id=$1`, run.ID)
	}
	return err
}

func sandboxStep(payload map[string]any) (map[string]any, string, error) {
	status, _ := payload["status"].(string)
	payload["current_server_time"] = fmt.Sprint(time.Now().Unix())
	if status != "ASSIGNED" && status != "IN PROCESS" {
		return nil, "", fmt.Errorf("unsupported active status %q", status)
	}
	points, err := orderPoints(payload)
	if err != nil {
		return nil, "", err
	}
	if len(points) == 0 {
		return nil, "", fmt.Errorf("trip has no path points")
	}
	if data := advancePathPoint(status, points); data != nil {
		payload["path"] = points
		return data, "point_status_changed", nil
	}
	next := "COMPLETED"
	if status == "ASSIGNED" {
		next = "IN PROCESS"
	}
	payload["status"] = next
	return map[string]any{"status": next}, "order_status_changed", nil
}

func advancePathPoint(status string, points []Point) map[string]any {
	for index := range points {
		point := &points[index]
		if point.Status == "COMPLETED" || point.Status == "FAILED" {
			continue
		}
		if status == "ASSIGNED" && index > 0 {
			break
		}
		applyPointStatus(point, nextSandboxPointStatus(point.Status))
		return map[string]any{"point_id": point.ID, "point_status": point.Status}
	}
	return nil
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
