package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type simulationState struct {
	Scenario          string   `json:"scenario"`
	Attempts          int      `json:"attempts"`
	ExcludedDrivers   []string `json:"excluded_drivers"`
	DisruptionHandled bool     `json:"disruption_handled"`
	Event             string   `json:"event"`
	Reason            string   `json:"reason,omitempty"`
}

func validScenario(input CreateOrderInput, environment string) error {
	if input.SandboxScenario == "" {
		return nil
	}
	if environment != "sandbox" {
		return &httpError{Status: 400, Code: 400, Message: "sandbox_scenario is sandbox-only"}
	}
	switch input.SandboxScenario {
	case "normal", "no_drivers", "driver_reject", "driver_cancel":
		return nil
	case "payment_failed":
		if input.PaymentMethod == "partner_pay" {
			return nil
		}
		return &httpError{Status: 400, Code: 400,
			Message: "payment_failed requires payment_method partner_pay"}
	default:
		return &httpError{Status: 400, Code: 400,
			Message: "unsupported sandbox_scenario; use normal, no_drivers, " +
				"driver_reject, driver_cancel, or payment_failed"}
	}
}

func readSimulation(payload map[string]any) (simulationState, error) {
	state := simulationState{Scenario: "normal", ExcludedDrivers: []string{}}
	if payload["simulation"] == nil {
		return state, nil
	}
	encoded, err := json.Marshal(payload["simulation"])
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(encoded, &state); err != nil {
		return state, fmt.Errorf("decode sandbox simulation: %w", err)
	}
	return state, nil
}

func (a *API) dispatchSandbox(ctx context.Context, tx *sql.Tx,
	payload map[string]any, state *simulationState,
) (map[string]any, string, error) {
	state.Attempts++
	state.Event = "searching"
	var driver *simulatedDriver
	var err error
	if state.Scenario != "no_drivers" {
		driver, err = a.reserveDriver(ctx, tx, payload, "sandbox", state.ExcludedDrivers)
		if err != nil {
			return nil, "", err
		}
	}
	if driver == nil {
		if state.Attempts >= 3 {
			cancelSimulation(payload, state, "NO_DRIVERS")
		}
		return map[string]any{"status": payload["status"]}, "order_status_changed", nil
	}
	if state.Scenario == "driver_reject" && !state.DisruptionHandled {
		state.ExcludedDrivers = append(state.ExcludedDrivers, driver.ID)
		state.DisruptionHandled, state.Event = true, "driver_rejected"
		return map[string]any{"status": "FINDING"}, "order_status_changed", nil
	}
	assignDriver(payload, *driver)
	payload["status"] = "ASSIGNED"
	state.Event = "assigned"
	return map[string]any{"status": "ASSIGNED"}, "order_status_changed", nil
}

func cancelSimulation(payload map[string]any, state *simulationState, reason string) {
	code := 9001
	if reason == "PAYMENT_FAILED" {
		code = 9002
	}
	payload["status"] = "CANCELLED"
	payload["cancel_code"] = map[string]any{"code": code, "message": reason}
	state.Reason, state.Event = reason, "cancelled"
}

func retryCancelledDriver(payload map[string]any, state *simulationState) error {
	encoded, err := json.Marshal(payload["driver"])
	if err != nil {
		return err
	}
	var driver struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(encoded, &driver); err != nil {
		return err
	}
	state.ExcludedDrivers = append(state.ExcludedDrivers, driver.ID)
	state.DisruptionHandled, state.Event = true, "driver_cancelled"
	payload["status"] = "FINDING"
	payload["driver"], payload["vehicle"] = map[string]any{}, map[string]any{}
	payload["driver_lock_time"], payload["eta_pickup"] = "0", 0
	return nil
}

func (a *API) advanceSimulation(ctx context.Context, tx *sql.Tx, payload map[string]any,
) (map[string]any, string, error) {
	payload["current_server_time"] = fmt.Sprint(time.Now().Unix())
	state, err := readSimulation(payload)
	if err != nil {
		return nil, "", err
	}
	defer func() { payload["simulation"] = state }()
	switch payload["status"] {
	case "WAITING_FOR_PAYMENT":
		simulatePayment(payload, &state)
		return map[string]any{"status": payload["status"]}, "order_status_changed", nil
	case "SCHEDULING", "FINDING":
		payload["status"] = "FINDING"
		return a.dispatchSandbox(ctx, tx, payload, &state)
	case "ASSIGNED":
		if state.Scenario == "driver_cancel" && !state.DisruptionHandled {
			if err := retryCancelledDriver(payload, &state); err != nil {
				return nil, "", err
			}
			return map[string]any{"status": "FINDING"}, "order_status_changed", nil
		}
	}
	return sandboxStep(payload)
}

func simulatePayment(payload map[string]any, state *simulationState) {
	payload["status"], state.Event = "FINDING", "payment_accepted"
	if state.Scenario == "payment_failed" {
		cancelSimulation(payload, state, "PAYMENT_FAILED")
	}
}

func (a *API) writeOperationError(w http.ResponseWriter, operation string, err error) {
	if failure, ok := err.(*httpError); ok {
		writeError(w, failure.Status, failure.Code, failure.Message)
		return
	}
	a.dbError(w, operation, err)
}
