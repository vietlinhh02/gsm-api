package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"gsm-api/internal/store"
)

func TestRideAPIEndToEnd(t *testing.T) {
	h := newIntegrationHarness(t)
	productionToken := getToken(t, h.server.URL, h.clientID, h.secret, false)
	pickup, dropoff := testPoints()
	orderID := runTripFlow(t, h.server.URL, productionToken, pickup, dropoff)
	runHourFlow(t, h.server.URL, productionToken, pickup)
	runGreenConnectFlow(t, h.server.URL, productionToken, pickup, dropoff)
	runGeocodeFlow(t, h.server.URL, productionToken)
	runSandboxFlow(t, h, productionToken, orderID, pickup, dropoff)
}

type integrationHarness struct {
	server   *httptest.Server
	provider *httptest.Server
	service  *API
	clientID string
	secret   string
}

func newIntegrationHarness(t *testing.T) *integrationHarness {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL-backed API flow")
	}
	base, err := store.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "test_" + fmt.Sprint(time.Now().UnixNano())
	if _, err := base.DB.Exec(`CREATE SCHEMA ` + schema); err != nil {
		base.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := base.DB.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); err != nil {
			t.Errorf("drop isolated test schema: %v", err)
		}
		base.Close()
	})
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsedURL.Query()
	query.Set("search_path", schema)
	parsedURL.RawQuery = query.Encode()
	databaseURL = parsedURL.String()
	provider := newOSMFixture()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	db, err := store.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
		provider.Close()
	})
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	clientID := "test-" + fmt.Sprint(time.Now().UnixNano())
	secret := "integration-test-secret"
	service := New(db, Config{
		ClientID: clientID, ClientSecret: secret,
		NominatimURL: provider.URL, OSRMURL: provider.URL,
	}, log.New(io.Discard, "", 0))
	service.OSM = NewOSM(provider.URL, provider.URL, service.HTTP)
	server := httptest.NewServer(service.Handler())
	t.Cleanup(server.Close)
	return &integrationHarness{
		server:   server,
		provider: provider,
		service:  service,
		clientID: clientID,
		secret:   secret,
	}
}

func newOSMFixture() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/route/v1/driving/"):
			io.WriteString(w, `{"code":"Ok","routes":[{"distance":4800,`+
				`"duration":600}]}`)
		case r.URL.Path == "/search":
			io.WriteString(
				w,
				`[{"osm_type":"way","osm_id":123,"lat":"10.7","lon":"106.6",`+
					`"display_name":"Test Road, Ho Chi Minh City","category":"highway",`+
					`"type":"residential","importance":0.5,"address":{"road":"Test Road",`+
					`"suburb":"Ward 1","city_district":"District 1",`+
					`"city":"Ho Chi Minh City"}}]`,
			)
		case r.URL.Path == "/reverse":
			io.WriteString(
				w,
				`{"osm_type":"way","osm_id":123,"lat":"10.7","lon":"106.6",`+
					`"display_name":"Test Road, Ho Chi Minh City","category":"highway",`+
					`"type":"residential","importance":0.5,"address":{"road":"Test Road",`+
					`"suburb":"Ward 1","city_district":"District 1",`+
					`"city":"Ho Chi Minh City"}}`,
			)
		default:
			http.NotFound(w, r)
		}
	}))
}

func testPoints() (Point, Point) {
	pickup := Point{
		Type:         "PICK UP",
		Latitude:     10.77653,
		Longitude:    106.70098,
		Name:         "Test Passenger",
		PhoneNumber:  "+84900000000",
		Address:      "Pickup Road",
		ShortAddress: "Pickup",
	}
	dropoff := Point{
		Type:         "DROP OFF",
		Latitude:     10.78011,
		Longitude:    106.70321,
		Name:         "Test Passenger",
		PhoneNumber:  "+84900000000",
		Address:      "Dropoff Road",
		ShortAddress: "Dropoff",
	}
	return pickup, dropoff
}

func runTripFlow(t *testing.T, baseURL, token string, pickup, dropoff Point) string {
	t.Helper()
	estimateBody := map[string]any{
		"service_ids": []any{map[string]any{"id": 2179}}, "order_time": 0,
		"payment_method": "cash", "points": []Point{pickup, dropoff},
	}
	estimateResponse := requestJSON(
		t,
		baseURL,
		http.MethodPost,
		"/v1/ride/estimate/order",
		token,
		estimateBody,
		http.StatusOK,
	)
	data := estimateResponse["data"].(map[string]any)
	item := data["service_estimates"].([]any)[0].(map[string]any)
	feeID := item["estimate_info"].(map[string]any)["fee_id"].(string)
	orderTime := parseInt64(t, data["order_time"].(string))
	createBody := map[string]any{
		"order_time": orderTime, "service_id": 2179, "service_type": "RIDE-TRIP",
		"payment_method": "cash", "fee_id": feeID, "points": []Point{pickup, dropoff},
	}
	created := requestJSON(
		t,
		baseURL,
		http.MethodPost,
		"/v1/ride/create-order",
		token,
		createBody,
		http.StatusOK,
	)
	orderID := created["data"].(map[string]any)["id"].(string)
	path := "/v1/ride/orders/" + orderID + "?extra_components=driver,path"
	order := requestJSON(
		t, baseURL, http.MethodGet, path, token, nil, http.StatusOK,
	)["data"].(map[string]any)
	if order["status"] != "FINDING" || len(order["path"].([]any)) != 2 {
		t.Fatalf("unexpected order detail: %#v", order)
	}
	list := requestJSON(
		t,
		baseURL,
		http.MethodGet,
		"/v1/ride/list-user-orders",
		token,
		nil,
		http.StatusOK,
	)
	if len(list["data"].([]any)) == 0 {
		t.Fatalf("order list omitted created order: %#v", list)
	}
	requestJSON(
		t,
		baseURL,
		http.MethodPost,
		"/v1/ride/cancel-order",
		token,
		map[string]any{"order_id": orderID, "cancel_code": 9000, "cancel_message": "test"},
		http.StatusOK,
	)
	requestJSON(
		t,
		baseURL,
		http.MethodPost,
		"/v1/ride/cancel-order",
		token,
		map[string]any{"order_id": orderID, "cancel_code": 9000, "cancel_message": "test"},
		http.StatusConflict,
	)
	return orderID
}

func runHourFlow(t *testing.T, baseURL, token string, pickup Point) {
	t.Helper()
	body := map[string]any{"data": []any{map[string]any{
		"id": "", "service_id": 1903, "hour": 2, "order_time": 0, "promo_code": "",
		"discount": 0, "payment_method": "cash", "expired_at": 0, "distance": 0,
		"points": []Point{pickup},
	}}}
	response := requestJSON(
		t,
		baseURL,
		http.MethodPost,
		"/v1/ride/estimate/hour-order",
		token,
		body,
		http.StatusOK,
	)
	item := response["data"].(map[string]any)["service_estimates"].([]any)[0].(map[string]any)
	feeID := item["estimate_infos"].([]any)[0].(map[string]any)["fee_id"]
	create := map[string]any{
		"order_time":     0,
		"service_id":     1903,
		"service_type":   "RIDE-HOUR",
		"payment_method": "cash",
		"fee_id":         feeID,
		"points":         []Point{pickup},
	}
	requestJSON(t, baseURL, http.MethodPost, "/v1/ride/create-order", token, create, http.StatusOK)
}

func runGreenConnectFlow(t *testing.T, baseURL, token string, pickup, dropoff Point) {
	t.Helper()
	body := map[string]any{
		"service_ids":    []any{map[string]any{"id": 2179}},
		"order_time":     0,
		"payment_method": "partner_pay",
		"points":         []Point{pickup, dropoff},
	}
	response := requestJSON(
		t,
		baseURL,
		http.MethodPost,
		"/v1/ride/estimate/order",
		token,
		body,
		http.StatusOK,
	)
	data := response["data"].(map[string]any)
	item := data["service_estimates"].([]any)[0].(map[string]any)
	points := []Point{pickup, dropoff}
	points[0].AddressNote = "Đón khách Test Passenger - 0900000000"
	points[1].AddressNote = "Trả khách Test Passenger - 0900000000"
	create := map[string]any{
		"order_time": parseInt64(t, data["order_time"].(string)), "service_id": 2179,
		"service_type": "RIDE-TRIP", "payment_method": "partner_pay",
		"partner_payment_method": "B2B", "fee_id": item["estimate_info"].(map[string]any)["fee_id"],
		"partner_trip_id": "TEST-TRIP-1", "points": points,
	}
	requestJSON(t, baseURL, http.MethodPost, "/v1/ride/create-order", token, create, http.StatusOK)
}

func runGeocodeFlow(t *testing.T, baseURL, token string) {
	t.Helper()
	geo := requestJSON(
		t,
		baseURL,
		http.MethodGet,
		"/v1/place/geocode?text=Test+Road",
		token,
		nil,
		http.StatusOK,
	)
	if geo["data"].([]any)[0].(map[string]any)["place_id"] != "W123" {
		t.Fatalf("unexpected geocode result: %#v", geo)
	}
	reverse := requestJSON(
		t,
		baseURL,
		http.MethodGet,
		"/v1/place/geocode?reverse=true&lat=10.7&lng=106.6",
		token,
		nil,
		http.StatusOK,
	)
	if len(reverse["data"].([]any)) != 1 {
		t.Fatalf("unexpected reverse geocode result: %#v", reverse)
	}
}

func runSandboxFlow(
	t *testing.T,
	h *integrationHarness,
	productionToken, productionOrderID string,
	pickup, dropoff Point,
) {
	t.Helper()
	token := getToken(t, h.server.URL, h.clientID, h.secret, true)
	requestJSON(
		t,
		h.server.URL,
		http.MethodGet,
		"/v1/sandbox/ride/orders/"+productionOrderID,
		token,
		nil,
		http.StatusNotFound,
	)
	if productionToken == token {
		t.Fatal("production and sandbox tokens should be distinct")
	}
	body := map[string]any{
		"service_ids":    []any{map[string]any{"id": 2179}},
		"order_time":     0,
		"payment_method": "cash",
		"points":         []Point{pickup, dropoff},
	}
	estimate := requestJSON(
		t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/estimate/order",
		token, body, http.StatusOK,
	)["data"].(map[string]any)
	item := estimate["service_estimates"].([]any)[0].(map[string]any)
	create := map[string]any{
		"order_time":     parseInt64(t, estimate["order_time"].(string)),
		"service_id":     2179,
		"service_type":   "RIDE-TRIP",
		"payment_method": "cash",
		"fee_id":         item["estimate_info"].(map[string]any)["fee_id"],
		"points":         []Point{pickup, dropoff},
	}
	created := requestJSON(
		t,
		h.server.URL,
		http.MethodPost,
		"/v1/sandbox/ride/create-order",
		token,
		create,
		http.StatusOK,
	)
	orderID := created["data"].(map[string]any)["id"].(string)
	path := "/v1/sandbox/ride/orders/" + orderID + "?extra_components=driver,path"
	order := requestJSON(
		t, h.server.URL, http.MethodGet, path, token, nil, http.StatusOK,
	)["data"].(map[string]any)
	pointID := order["path"].([]any)[0].(map[string]any)["id"].(string)
	received := setupSandboxWebhook(t, h)
	requestJSON(
		t,
		h.server.URL,
		http.MethodPost,
		"/v1/sandbox/ride/update-status",
		token,
		map[string]any{"update_type": "order", "order_id": orderID, "status": "ASSIGNED"},
		http.StatusOK,
	)
	requestJSON(
		t,
		h.server.URL,
		http.MethodPost,
		"/v1/sandbox/ride/update-status",
		token,
		map[string]any{
			"update_type":  "point",
			"order_id":     orderID,
			"point_id":     pointID,
			"point_status": "ARRIVED",
		},
		http.StatusOK,
	)
	if err := h.service.deliverOneWebhook(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case header := <-received:
		if header.Get("X-Environment") != "sandbox" {
			t.Fatalf("unexpected webhook environment: %#v", header)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("webhook callback was not delivered")
	}
	detail := requestJSON(
		t, h.server.URL, http.MethodGet, path, token, nil, http.StatusOK,
	)["data"].(map[string]any)
	if detail["status"] != "ASSIGNED" ||
		detail["driver"].(map[string]any)["id"] != "local-driver" ||
		detail["path"].([]any)[0].(map[string]any)["status"] != "ARRIVED" {
		t.Fatalf("sandbox status simulation failed: %#v", detail)
	}
	requestJSON(
		t,
		h.server.URL,
		http.MethodPost,
		"/v1/sandbox/ride/update-status",
		token,
		map[string]any{"update_type": "order", "order_id": orderID, "status": "IN PROCESS"},
		http.StatusOK,
	)
	requestJSON(
		t,
		h.server.URL,
		http.MethodPost,
		"/v1/sandbox/ride/cancel-order",
		token,
		map[string]any{"order_id": orderID, "cancel_code": 9000, "cancel_message": "too late"},
		http.StatusConflict,
	)
	requestJSON(
		t,
		h.server.URL,
		http.MethodGet,
		"/v1/ride/orders/"+orderID,
		productionToken,
		nil,
		http.StatusNotFound,
	)
}

func setupSandboxWebhook(t *testing.T, h *integrationHarness) <-chan http.Header {
	t.Helper()
	received := make(chan http.Header, 1)
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if r.Header.Get("X-VSF-Signature") != webhookSignature(body, "callback-key") {
			t.Error("webhook signature did not match raw callback body")
		}
		received <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(callback.Close)
	h.service.Config.WebhookURL = callback.URL
	h.service.Config.WebhookSignKey = "callback-key"
	return received
}

func getToken(t *testing.T, baseURL, clientID, secret string, sandbox bool) string {
	t.Helper()
	path, scope := "/v1/public/token", "ride.trips"
	if sandbox {
		path, scope = "/v1/sandbox/token", "sandbox.ride.trips"
	}
	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {secret},
		"grant_type":    {"client_credentials"},
		"scope":         {scope},
	}
	request, err := http.NewRequest(
		http.MethodPost,
		baseURL+path,
		bytes.NewBufferString(form.Encode()),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("token response status %d: %#v", response.StatusCode, result)
	}
	return result["access_token"].(string)
}

func requestJSON(
	t *testing.T,
	baseURL, method, path, token string,
	body any,
	wantStatus int,
) map[string]any {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, baseURL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf(
			"%s %s returned %d, wanted %d: %#v",
			method,
			path,
			response.StatusCode,
			wantStatus,
			result,
		)
	}
	return result
}

func parseInt64(t *testing.T, value string) int64 {
	t.Helper()
	var parsed int64
	if _, err := fmt.Sscan(value, &parsed); err != nil {
		t.Fatal(err)
	}
	return parsed
}
