package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func openOrderStream(t *testing.T, h *integrationHarness, token, id string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		h.server.URL+"/v1/sandbox/ride/orders/"+id+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { response.Body.Close() })
	return response
}

func readStreamOrder(t *testing.T, reader *bufio.Reader) map[string]any {
	t.Helper()
	line, err := reader.ReadString('\n')
	if err != nil || line != "event: order\n" {
		t.Fatalf("event=%q err=%v", line, err)
	}
	line, err = reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal([]byte(line[len("data: "):]), &event); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	return event.Data
}

func TestOrderStreamPushAndReconnect(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	id := createAutomaticSandboxTrip(t, h, token)
	response := openOrderStream(t, h, token, id)
	if response.StatusCode != http.StatusOK ||
		response.Header.Get("Content-Type") != "text/event-stream" ||
		response.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatal(response.Status, response.Header)
	}
	reader := bufio.NewReader(response.Body)
	if order := readStreamOrder(t, reader); order["status"] != "FINDING" {
		t.Fatal(order)
	}
	if _, err := h.service.Store.DB.Exec(
		`UPDATE sandbox_runs SET next_step_at=NOW() WHERE order_id=$1`, id,
	); err != nil {
		t.Fatal(err)
	}
	if advanced, err := h.service.advanceSandbox(context.Background()); err != nil || !advanced {
		t.Fatalf("advance=%v err=%v", advanced, err)
	}
	assigned := readStreamOrder(t, reader)
	if assigned["status"] != "ASSIGNED" ||
		assigned["driver"].(map[string]any)["id"] != "hcm-car-1" {
		t.Fatal(assigned)
	}
	pointID := assigned["path"].([]any)[0].(map[string]any)["id"]
	for _, status := range []string{"ARRIVING", "NEARED", "ARRIVED"} {
		requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/update-status", token,
			map[string]any{"update_type": "point", "order_id": id,
				"point_id": pointID, "point_status": status}, http.StatusOK)
		pointOrder := readStreamOrder(t, reader)
		if pointOrder["path"].([]any)[0].(map[string]any)["status"] != status {
			t.Fatal(pointOrder)
		}
	}
	response.Body.Close()
	requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/update-status", token,
		map[string]any{"update_type": "order", "order_id": id, "status": "IN PROCESS"}, http.StatusOK)
	response = openOrderStream(t, h, token, id)
	reader = bufio.NewReader(response.Body)
	if order := readStreamOrder(t, reader); order["status"] != "IN PROCESS" {
		t.Fatal(order)
	}
	requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/update-status", token,
		map[string]any{"update_type": "order", "order_id": id, "status": "COMPLETED"}, http.StatusOK)
	if order := readStreamOrder(t, reader); order["status"] != "COMPLETED" {
		t.Fatal(order)
	}
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("terminal stream stayed open")
	}
	response = openOrderStream(t, h, token, id)
	if order := readStreamOrder(t, bufio.NewReader(response.Body)); order["status"] != "COMPLETED" {
		t.Fatal(order)
	}
}

func TestOrderStreamAuthorization(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	legacy := "legacy-production-token"
	if _, err := h.service.Store.DB.Exec(`
 INSERT INTO access_tokens(token_hash,client_id,scope,expires_at) VALUES($1,$2,$3,$4)
 `, hashToken(legacy), h.clientID, "ride.trips", time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	id := createAutomaticSandboxTrip(t, h, token)
	foreign := "foreign-stream-token"
	if _, err := h.service.Store.DB.Exec(`
  INSERT INTO access_tokens(token_hash,client_id,scope,expires_at) VALUES($1,$2,$3,$4)
 `, hashToken(foreign), "another-client", "sandbox.ride.trips",
		time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		token string
		id    string
		code  int
	}{
		{"", id, 401}, {"invalid", id, 401}, {legacy, id, 401}, {foreign, id, 404},
		{token, "missing-order", 404},
	} {
		response := openOrderStream(t, h, item.token, item.id)
		if response.StatusCode != item.code {
			t.Fatalf("want %d got %d", item.code, response.StatusCode)
		}
		response.Body.Close()
	}
}

func TestOrderHubIsolationAndCleanup(t *testing.T) {
	a := &API{}
	key := orderStreamKey{"sandbox", "client", "trip"}
	changed, unsubscribe := a.orderHub.subscribe(key)
	for _, item := range []orderStreamKey{
		{"production", "client", "trip"}, {"sandbox", "other", "trip"}, {"sandbox", "client", "other"},
	} {
		a.publishOrder(item.environment, item.client, item.id)
	}
	select {
	case <-changed:
		t.Fatal("received another trip's change")
	default:
	}
	for range 100 {
		a.publishOrder("sandbox", "client", "trip")
	}
	select {
	case <-changed:
	default:
		t.Fatal("missing notification")
	}
	unsubscribe()
	if len(a.orderHub.subscribers) != 0 {
		t.Fatal("subscriber was not removed")
	}
}

func TestOrderStreamCancellationUnsubscribes(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	id := createAutomaticSandboxTrip(t, h, token)
	response := openOrderStream(t, h, token, id)
	readStreamOrder(t, bufio.NewReader(response.Body))
	response.Body.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		h.service.orderHub.mu.Lock()
		count := len(h.service.orderHub.subscribers)
		h.service.orderHub.mu.Unlock()
		if count == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("disconnected stream retained its subscription")
}
