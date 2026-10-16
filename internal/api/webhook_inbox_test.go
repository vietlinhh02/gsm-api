package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func createInbox(t *testing.T, h *integrationHarness, token string) map[string]any {
	t.Helper()
	return requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/webhook-inboxes",
		token, map[string]any{}, http.StatusCreated)["data"].(map[string]any)
}

func TestWebhookInboxReceivesAndIsolatesRequests(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	inbox := createInbox(t, h, token)
	path := inbox["receive_path"].(string)
	body := `{"message":"Xin chào","event_id":"external"}`
	postWebhookBody(t, h.server.URL+path, body, 200)
	metadataPath := "/v1/sandbox/webhook-inboxes/" + inbox["id"].(string)
	metadata := requestJSON(t, h.server.URL, http.MethodGet, metadataPath, token, nil, 200)
	receipts := metadata["receipts"].([]any)
	if len(receipts) != 1 || receipts[0].(map[string]any)["preview"] != body {
		t.Fatal(metadata)
	}
	id := receipts[0].(map[string]any)["id"].(float64)
	receipt := requestJSON(t, h.server.URL, http.MethodGet,
		fmt.Sprintf("%s/receipts/%.0f", metadataPath, id), token, nil, 200)["data"].(map[string]any)
	if receipt["body"] != body || receipt["headers"].(map[string]any)["Authorization"] != nil {
		t.Fatal("receipt altered the raw body or exposed authorization")
	}
	requestJSON(t, h.server.URL, http.MethodGet, metadataPath, "", nil, 401)
	requestJSON(t, h.server.URL, http.MethodGet, metadataPath+"/receipts/abc", token, nil, 400)
	requestJSON(t, h.server.URL, http.MethodGet, metadataPath+"/receipts/999999", token, nil, 404)
	assertOtherClientCannotUseInbox(t, h, inbox)
}

func assertOtherClientCannotUseInbox(t *testing.T, h *integrationHarness, inbox map[string]any) {
	t.Helper()
	other := "other-client-token"
	_, err := h.service.Store.DB.Exec(`INSERT INTO access_tokens(token_hash,client_id,scope,expires_at)
		VALUES($1,'other-client','sandbox.ride.trips',9999999999)`, hashToken(other))
	if err != nil {
		t.Fatal(err)
	}
	requestJSON(t, h.server.URL, http.MethodGet,
		"/v1/sandbox/webhook-inboxes/"+inbox["id"].(string), other, nil, 404)
	body := scenarioBooking(t, h, other, "normal")
	body["webhook_inbox_id"] = inbox["id"]
	requestJSON(t, h.server.URL, http.MethodPost,
		"/v1/sandbox/ride/create-order", other, body, 404)
}

func postWebhookBody(t *testing.T, target, body string, status int) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer should-not-be-stored")
	request.Header.Set("X-Custom-Source", "external-system")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		t.Fatalf("receiver status=%d, want=%d", response.StatusCode, status)
	}
}

func TestInboxWebhookRetriesAndSignatures(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	inbox := createInbox(t, h, token)
	body := scenarioBooking(t, h, token, "normal")
	body["webhook_inbox_id"] = inbox["id"]
	for range 2 {
		requestJSON(t, h.server.URL, http.MethodPost,
			"/v1/sandbox/ride/create-order", token, body, 200)
	}
	h.service.Config.WebhookSignKey = "inbox-signing-key"
	h.service.Config.WebhookTokenURL = "http://127.0.0.1:1/not-used-for-inboxes"
	var eventID string
	if err := h.service.Store.DB.QueryRow(`SELECT event_id FROM webhook_events`).
		Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Store.DB.Exec(`UPDATE webhook_events
		SET target_url='/webhook-inbox/missing-token'`); err != nil {
		t.Fatal(err)
	}
	if err := h.service.deliverOneWebhook(context.Background()); err == nil {
		t.Fatal("missing receiver should fail delivery")
	}
	assertWebhookRetryScheduled(t, h)
	if _, err := h.service.Store.DB.Exec(`UPDATE webhook_events
		SET target_url=$1,next_attempt_at=NOW()`, inbox["receive_path"]); err != nil {
		t.Fatal(err)
	}
	if err := h.service.deliverOneWebhook(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertSignedReceipt(t, h, inbox["id"], eventID)
}

func assertWebhookRetryScheduled(t *testing.T, h *integrationHarness) {
	t.Helper()
	var attempts int
	var pending bool
	if err := h.service.Store.DB.QueryRow(`SELECT attempts,
		delivered_at IS NULL AND next_attempt_at>NOW() FROM webhook_events`).
		Scan(&attempts, &pending); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || !pending {
		t.Fatal("failed callback did not schedule a retry")
	}
}

func assertSignedReceipt(t *testing.T, h *integrationHarness, inboxID any, eventID string) {
	t.Helper()
	var body string
	var headers map[string]string
	if err := h.service.Store.DB.QueryRow(`SELECT body,headers
		FROM webhook_receipts WHERE inbox_id=$1`, inboxID).
		Scan(&body, jsonScanner{target: &headers}); err != nil {
		t.Fatal(err)
	}
	if headers[http.CanonicalHeaderKey("X-VSF-Signature")] !=
		webhookSignature([]byte(body), "inbox-signing-key") {
		t.Fatal("signature was not calculated over the received raw body")
	}
	var event capturedWebhook
	if err := json.Unmarshal([]byte(body), &event); err != nil {
		t.Fatal(err)
	}
	if event.EventID != eventID {
		t.Fatal("callback retry changed the event ID")
	}
	var attempts int
	var delivered bool
	if err := h.service.Store.DB.QueryRow(`SELECT attempts,delivered_at IS NOT NULL
		FROM webhook_events WHERE event_id=$1`, eventID).Scan(&attempts, &delivered); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || !delivered {
		t.Fatal("successful retry was not acknowledged")
	}
}

func TestWebhookInboxBoundsAndRawBodies(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret)
	inbox := createInbox(t, h, token)
	target := h.server.URL + inbox["receive_path"].(string)
	postWebhookBody(t, target, strings.Repeat("x", maxWebhookBody+1), 413)
	postWebhookBody(t, target, string([]byte{0xff}), 400)
	postWebhookBody(t, target, "null\x00byte", 400)
	postWebhookBody(t, h.server.URL+"/webhook-inbox/unknown", "{}", 404)
	postWebhookBody(t, target, strings.Repeat("x", maxWebhookBody), 200)
	for index := range 100 {
		postWebhookBody(t, target, fmt.Sprintf("not JSON %d", index), 200)
	}
	metadata := requestJSON(t, h.server.URL, http.MethodGet,
		"/v1/sandbox/webhook-inboxes/"+inbox["id"].(string), token, nil, 200)
	receipts := metadata["receipts"].([]any)
	if len(receipts) != 100 || receipts[0].(map[string]any)["preview"] != "not JSON 99" {
		t.Fatal("inbox did not retain the latest 100 receipts")
	}
}

func TestWebhookInboxCapturesScenarioSnapshots(t *testing.T) {
	cases := []struct {
		scenario, reason string
		steps            int
	}{
		{"normal", "", 11}, {"no_drivers", "NO_DRIVERS", 3},
		{"driver_reject", "", 12}, {"driver_cancel", "", 13},
		{"payment_failed", "PAYMENT_FAILED", 1},
	}
	for _, tc := range cases {
		t.Run(tc.scenario, func(t *testing.T) {
			h := newIntegrationHarness(t)
			token := getToken(t, h.server.URL, h.clientID, h.secret)
			inbox := createInbox(t, h, token)
			body := scenarioBooking(t, h, token, tc.scenario)
			body["webhook_inbox_id"] = inbox["id"]
			created := requestJSON(t, h.server.URL, http.MethodPost,
				"/v1/sandbox/ride/create-order", token, body, 200)["data"].(map[string]any)
			for range tc.steps {
				advanceTrip(t, h, created["id"].(string))
			}
			for range tc.steps + 1 {
				if err := h.service.deliverOneWebhook(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			assertInboxScenario(t, h, inbox, tc.scenario, tc.reason)
		})
	}
}

type capturedWebhook struct {
	EventID string `json:"event_id"`
	Data    struct {
		Status     string          `json:"status"`
		Simulation simulationState `json:"simulation"`
	} `json:"data"`
}

func readInboxEvents(t *testing.T, h *integrationHarness, id any) []capturedWebhook {
	t.Helper()
	rows, err := h.service.Store.DB.Query(`SELECT body FROM webhook_receipts
  WHERE inbox_id=$1 ORDER BY id`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	events := []capturedWebhook{}
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			t.Fatal(err)
		}
		var event capturedWebhook
		if err := json.Unmarshal([]byte(body), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return events
}

func assertInboxScenario(t *testing.T, h *integrationHarness, inbox map[string]any,
	scenario, reason string,
) {
	t.Helper()
	events := readInboxEvents(t, h, inbox["id"])
	if len(events) == 0 {
		t.Fatal("webhook inbox did not receive events")
	}
	observed := map[string]bool{}
	reasons := map[string]bool{}
	for _, event := range events {
		observed[event.Data.Simulation.Event] = true
		reasons[event.Data.Simulation.Reason] = true
	}
	if !reasons[reason] {
		t.Fatalf("missing webhook reason %q", reason)
	}
	required := map[string]string{"driver_reject": "driver_rejected",
		"driver_cancel": "driver_cancelled"}
	if event := required[scenario]; event != "" && !observed[event] {
		t.Fatalf("transient scenario event %q was not captured", event)
	}
}
