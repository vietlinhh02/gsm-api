package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestSandboxTripSteps(t *testing.T) {
	for _, mode := range []string{"Car", "Motorcycle"} {
		t.Run(mode, func(t *testing.T) {
			payload := map[string]any{"status": "ASSIGNED", "travel_mode": mode, "path": []Point{
				{ID: "pickup", Status: "CREATED", Latitude: 21, Longitude: 105},
				{ID: "dropoff", Status: "CREATED", Latitude: 22, Longitude: 106},
			}}
			var statuses []string
			var points []string
			for range 10 {
				data, event, err := sandboxStep(payload)
				if err != nil {
					t.Fatal(err)
				}
				if event == "order_status_changed" {
					statuses = append(statuses, data["status"].(string))
				} else {
					points = append(points, data["point_id"].(string)+":"+data["point_status"].(string))
				}
			}
			if !reflect.DeepEqual(statuses, []string{"IN PROCESS", "COMPLETED"}) {
				t.Fatalf("unexpected status sequence: %v", statuses)
			}
			want := []string{"pickup:ARRIVING", "pickup:NEARED", "pickup:ARRIVED", "pickup:COMPLETED",
				"dropoff:ARRIVING", "dropoff:NEARED", "dropoff:ARRIVED", "dropoff:COMPLETED"}
			if !reflect.DeepEqual(points, want) {
				t.Fatalf("unexpected path sequence: %v", points)
			}
			path := payload["path"].([]Point)
			for _, point := range path {
				if point.ArriveTime == "" || point.ProofOfWork == nil ||
					point.ProofOfWork.Latitude != point.Latitude {
					t.Fatalf("missing arrival/proof: %#v", point)
				}
			}
		})
	}
}

func TestSandboxInvalidTrips(t *testing.T) {
	for _, payload := range []map[string]any{
		{"status": "CANCELLED"}, {"status": "COMPLETED"}, {"status": "UNKNOWN"},
		{"status": "ASSIGNED", "path": []Point{}}, {"status": "IN PROCESS", "path": "invalid"},
	} {
		if _, _, err := sandboxStep(payload); err == nil {
			t.Fatalf("expected error for %#v", payload)
		}
	}
}

func TestSandboxAutomaticWebhooks(t *testing.T) {
	h := newIntegrationHarness(t)
	received := make(chan map[string]any, 20)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		if r.Header.Get("X-Environment") != "sandbox" ||
			r.Header.Get("X-VSF-Signature") != webhookSignature(body, "test-key") {
			t.Error("incorrect environment or signature")
		}
		var event map[string]any
		if err := json.Unmarshal(body, &event); err != nil {
			t.Error(err)
		}
		received <- event
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()
	h.service.Config.WebhookURL = receiver.URL
	h.service.Config.WebhookSignKey = "test-key"
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	id := createAutomaticSandboxTrip(t, h, token)
	ctx := context.Background()
	for range 11 {
		if _, err := h.service.Store.DB.Exec(
			`UPDATE sandbox_runs SET next_step_at=NOW() WHERE order_id=$1`, id,
		); err != nil {
			t.Fatal(err)
		}
		advanced, err := h.service.advanceSandbox(ctx)
		if err != nil || !advanced {
			t.Fatalf("advance=%v err=%v", advanced, err)
		}
	}
	for range 12 {
		if err := h.service.deliverOneWebhook(ctx); err != nil {
			t.Fatal(err)
		}
		select {
		case event := <-received:
			if event["resource_href"] != h.server.URL+"/v1/sandbox/ride/orders/"+id {
				t.Fatalf("wrong resource: %v", event)
			}
		case <-time.After(time.Second):
			t.Fatal("webhook was not delivered")
		}
	}
	detail := requestJSON(t, h.server.URL, http.MethodGet,
		"/v1/sandbox/ride/orders/"+id+"?extra_components=driver,path", token, nil, http.StatusOK)
	if detail["data"].(map[string]any)["status"] != "COMPLETED" {
		t.Fatal(detail)
	}
}

func createAutomaticSandboxTrip(t *testing.T, h *integrationHarness, token string) string {
	t.Helper()
	pickup, dropoff := testPoints()
	points := []Point{pickup, dropoff}
	estimate := requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/estimate/order", token,
		map[string]any{"service_ids": []any{map[string]any{"id": 2179}}, "order_time": 0,
			"payment_method": "cash", "points": points}, http.StatusOK)["data"].(map[string]any)
	item := estimate["service_estimates"].([]any)[0].(map[string]any)
	created := requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/create-order", token,
		map[string]any{"service_id": 2179, "service_type": "RIDE-TRIP", "payment_method": "cash",
			"order_time": parseInt64(t, estimate["order_time"].(string)), "points": points,
			"fee_id": item["estimate_info"].(map[string]any)["fee_id"]}, http.StatusOK)
	return created["data"].(map[string]any)["id"].(string)
}

func TestSandboxCancellationStopsWorker(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	id := createAutomaticSandboxTrip(t, h, token)
	requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/cancel-order", token,
		map[string]any{"order_id": id, "cancel_code": 9000, "cancel_message": "test"}, http.StatusOK)
	if _, err := h.service.Store.DB.Exec(
		`UPDATE sandbox_runs SET next_step_at=NOW() WHERE order_id=$1`, id,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.advanceSandbox(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := h.service.Store.DB.QueryRow(
		`SELECT COUNT(*) FROM sandbox_runs WHERE order_id=$1`, id,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("cancelled trip still scheduled")
	}
}

func TestSandboxResumesWithoutWebhookAndHonorsSchedule(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	id := createAutomaticSandboxTrip(t, h, token)
	if _, err := h.service.Store.DB.Exec(`
  UPDATE sandbox_runs SET next_step_at=NOW()+INTERVAL '1 hour' WHERE order_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if advanced, err := h.service.advanceSandbox(context.Background()); err != nil || advanced {
		t.Fatalf("future trip advanced=%v err=%v", advanced, err)
	}
	if _, err := h.service.Store.DB.Exec(
		`UPDATE sandbox_runs SET next_step_at=NOW() WHERE order_id=$1`, id,
	); err != nil {
		t.Fatal(err)
	}
	restarted := New(h.service.Store, h.service.Config, h.service.Logger)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); restarted.RunSandboxWorker(ctx) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		if err := h.service.Store.DB.QueryRow(
			`SELECT status FROM orders WHERE id=$1`, id,
		).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "ASSIGNED" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("restarted worker did not resume pending sandbox trip without a webhook URL")
}
