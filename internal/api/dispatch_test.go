package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func scenarioBooking(t *testing.T, h *integrationHarness, token, scenario string) map[string]any {
	t.Helper()
	mode := "cash"
	if scenario == "payment_failed" {
		mode = "partner_pay"
	}
	pickup, dropoff := testPoints()
	points := []Point{pickup, dropoff}
	estimate := requestJSON(t, h.server.URL, http.MethodPost,
		"/v1/sandbox/ride/estimate/order", token,
		map[string]any{"service_ids": []any{map[string]any{"id": 2179}},
			"order_time": 0, "payment_method": mode, "points": points},
		http.StatusOK)["data"].(map[string]any)
	item := estimate["service_estimates"].([]any)[0].(map[string]any)
	return map[string]any{"service_id": 2179, "service_type": "RIDE-TRIP",
		"fee_id":         item["estimate_info"].(map[string]any)["fee_id"],
		"order_time":     parseInt64(t, estimate["order_time"].(string)),
		"payment_method": mode, "points": points, "sandbox_scenario": scenario}
}

func advanceTrip(t *testing.T, h *integrationHarness, id string) map[string]any {
	t.Helper()
	if _, err := h.service.Store.DB.Exec(
		`UPDATE sandbox_runs SET next_step_at=NOW() WHERE order_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if advanced, err := h.service.advanceSandbox(context.Background()); err != nil || !advanced {
		t.Fatalf("trip did not advance: advanced=%v error=%v", advanced, err)
	}
	var payload map[string]any
	if err := h.service.Store.DB.QueryRow(`SELECT payload FROM orders WHERE id=$1`, id).
		Scan(jsonScanner{target: &payload}); err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestDispatchScenarios(t *testing.T) {
	cases := []struct {
		scenario, status, reason, driver string
		steps                            int
	}{
		{"normal", "COMPLETED", "", "hcm-car-1", 11},
		{"no_drivers", "CANCELLED", "NO_DRIVERS", "", 3},
		{"driver_reject", "COMPLETED", "", "hcm-car-2", 12},
		{"driver_cancel", "COMPLETED", "", "hcm-car-2", 13},
		{"payment_failed", "CANCELLED", "PAYMENT_FAILED", "", 1},
	}
	for _, tc := range cases {
		t.Run(tc.scenario, func(t *testing.T) {
			h := newIntegrationHarness(t)
			token := getToken(t, h.server.URL, h.clientID, h.secret)
			body := scenarioBooking(t, h, token, tc.scenario)
			payload := requestJSON(t, h.server.URL, http.MethodPost,
				"/v1/sandbox/ride/create-order", token, body, http.StatusOK)["data"].(map[string]any)
			id := payload["id"].(string)
			for range tc.steps {
				payload = advanceTrip(t, h, id)
			}
			state, err := readSimulation(payload)
			if err != nil || payload["status"] != tc.status || state.Reason != tc.reason {
				t.Fatalf("unexpected final trip: %v, error=%v", payload, err)
			}
			if driver, _ := payload["driver"].(map[string]any)["id"].(string); driver != tc.driver {
				t.Fatalf("driver=%q, want=%q", driver, tc.driver)
			}
			assertReleasedTrip(t, h)
		})
	}
}

func assertReleasedTrip(t *testing.T, h *integrationHarness) {
	t.Helper()
	var pending, occupied int
	if err := h.service.Store.DB.QueryRow(`SELECT COUNT(*) FROM sandbox_runs`).
		Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if err := h.service.Store.DB.QueryRow(`SELECT COUNT(*) FROM orders
  WHERE driver_id IS NOT NULL AND status IN ('ASSIGNED','IN PROCESS')`).
		Scan(&occupied); err != nil {
		t.Fatal(err)
	}
	if pending != 0 || occupied != 0 {
		t.Fatalf("finished trip holds resources: scheduled=%d, occupied=%d", pending, occupied)
	}
}

func TestFleetAvailabilityAndRelease(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	ids := []string{}
	for range 3 {
		body := scenarioBooking(t, h, token, "normal")
		payload := requestJSON(t, h.server.URL, http.MethodPost,
			"/v1/sandbox/ride/create-order", token, body, http.StatusOK)["data"].(map[string]any)
		ids = append(ids, payload["id"].(string))
	}
	for index, id := range ids {
		payload := advanceTrip(t, h, id)
		want := []string{"ASSIGNED", "ASSIGNED", "FINDING"}
		if payload["status"] != want[index] {
			t.Fatal(payload)
		}
	}
	assertFleetCount(t, h, token, 0)
	requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/cancel-order", token,
		map[string]any{"order_id": ids[0], "cancel_code": 9000, "cancel_message": "test"}, 200)
	assertFleetCount(t, h, token, 1)
	payload := advanceTrip(t, h, ids[2])
	if payload["status"] != "ASSIGNED" ||
		payload["driver"].(map[string]any)["id"] != "hcm-car-1" {
		t.Fatalf("freed driver was not reassigned: %v", payload)
	}
}

func assertFleetCount(t *testing.T, h *integrationHarness, token string, count float64) {
	t.Helper()
	pickup, _ := testPoints()
	response := requestJSON(t, h.server.URL, http.MethodPost,
		"/v1/sandbox/ride/list-available-services", token,
		map[string]any{"pickups": []Point{pickup}}, 200)
	for _, item := range response["data"].([]any) {
		service := item.(map[string]any)["service"].(map[string]any)
		if service["travel_mode"] == "Car" &&
			(service["available_vehicles"] != count || service["enable"] != (count > 0)) {
			t.Fatalf("incorrect fleet availability: %v", service)
		}
	}
}

func TestBookingRetriesAndConflicts(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	body := scenarioBooking(t, h, token, "normal")
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		id  string
		err error
	}
	results := make(chan result, 5)
	for range 5 {
		go func() {
			id, err := sendBooking(h.server.URL, token, encoded)
			results <- result{id, err}
		}()
	}
	id := ""
	ids := map[string]bool{}
	for range 5 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		id = result.id
		ids[id] = true
	}
	if len(ids) != 1 {
		t.Fatal("concurrent retry created another order")
	}
	if _, err := h.service.Store.DB.Exec(`UPDATE estimates SET expires_at=0 WHERE fee_id=$1`,
		body["fee_id"]); err != nil {
		t.Fatal(err)
	}
	retried := requestJSON(t, h.server.URL, http.MethodPost,
		"/v1/sandbox/ride/create-order", token, body, 200)["data"].(map[string]any)
	if retried["id"] != id {
		t.Fatal("retry after expiration lost the already accepted booking")
	}
	body["sandbox_scenario"] = "driver_cancel"
	requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/create-order", token, body, 409)
}

func sendBooking(origin, token string, body []byte) (string, error) {
	request, err := http.NewRequest(http.MethodPost, origin+"/v1/sandbox/ride/create-order",
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("booking returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	err = json.NewDecoder(response.Body).Decode(&result)
	return result.Data.ID, err
}

func TestFleetDisabledAndOutsideRadius(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	if _, err := h.service.Store.DB.Exec(
		`UPDATE simulated_drivers SET enabled=FALSE WHERE id='hcm-car-1'`); err != nil {
		t.Fatal(err)
	}
	assertFleetCount(t, h, token, 1)
	id := createAutomaticSandboxTrip(t, h, token)
	payload := advanceTrip(t, h, id)
	if payload["driver"].(map[string]any)["id"] != "hcm-car-2" {
		t.Fatal("disabled driver was assigned")
	}
	response := requestJSON(t, h.server.URL, http.MethodPost,
		"/v1/sandbox/ride/list-available-services", token,
		map[string]any{"pickups": []Point{{Latitude: 16.06, Longitude: 108.22}}}, 200)
	for _, item := range response["data"].([]any) {
		service := item.(map[string]any)["service"].(map[string]any)
		if service["enable"] != false || service["eta_pickup"] != float64(0) {
			t.Fatalf("out-of-range vehicle advertised as available: %v", service)
		}
	}
}

func TestBookingAndTransitionErrors(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	body := scenarioBooking(t, h, token, "normal")
	for _, scenario := range []string{"unknown", "payment_failed"} {
		body["sandbox_scenario"] = scenario
		requestJSON(t, h.server.URL, http.MethodPost,
			"/v1/sandbox/ride/create-order", token, body, 400)
	}
	body["sandbox_scenario"] = "normal"
	if _, err := h.service.Store.DB.Exec(`UPDATE estimates SET expires_at=0 WHERE fee_id=$1`,
		body["fee_id"]); err != nil {
		t.Fatal(err)
	}
	requestJSON(t, h.server.URL, http.MethodPost,
		"/v1/sandbox/ride/create-order", token, body, 400)
	id := createAutomaticSandboxTrip(t, h, token)
	requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/update-status", token,
		map[string]any{"update_type": "order", "order_id": id, "status": "COMPLETED"}, 409)
	payload := advanceTrip(t, h, id)
	pointID := payload["path"].([]any)[0].(map[string]any)["id"]
	requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/update-status", token,
		map[string]any{"update_type": "point", "order_id": id,
			"point_id": pointID, "point_status": "COMPLETED"}, 409)
	for range 2 {
		requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/cancel-order", token,
			map[string]any{"order_id": id, "cancel_code": 9000, "cancel_message": "test"}, 200)
	}
	requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/update-status", token,
		map[string]any{"update_type": "order", "order_id": id, "status": "ASSIGNED"}, 409)
}

func TestConcurrentAssignmentUsesDistinctDrivers(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	ids := []string{createAutomaticSandboxTrip(t, h, token),
		createAutomaticSandboxTrip(t, h, token)}
	start := make(chan struct{})
	errors := make(chan error, 2)
	for _, id := range ids {
		go func() {
			<-start
			errors <- sendAssignment(h.server.URL, token, id)
		}()
	}
	close(start)
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	var distinct int
	if err := h.service.Store.DB.QueryRow(`SELECT COUNT(DISTINCT driver_id) FROM orders
		WHERE status='ASSIGNED'`).Scan(&distinct); err != nil {
		t.Fatal(err)
	}
	if distinct != 2 {
		t.Fatalf("concurrent assignment did not use two distinct drivers: %d", distinct)
	}
}

func sendAssignment(origin, token, id string) error {
	body, err := json.Marshal(map[string]any{"update_type": "order",
		"order_id": id, "status": "ASSIGNED"})
	if err != nil {
		return err
	}
	request, err := http.NewRequest(http.MethodPost, origin+"/v1/sandbox/ride/update-status",
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("assignment returned HTTP %d", response.StatusCode)
	}
	return nil
}

func TestMotorcycleAddonReturnsClientError(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	pickup, dropoff := testPoints()
	requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/estimate/order", token,
		map[string]any{"service_ids": []any{map[string]any{"id": 2180,
			"addons": []Addon{{ID: 10, Apply: true}}}}, "order_time": 0,
			"payment_method": "cash", "points": []Point{pickup, dropoff}}, 400)
}
