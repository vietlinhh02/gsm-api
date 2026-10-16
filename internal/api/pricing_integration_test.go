package api

import (
	"net/http"
	"testing"
)

func TestHanoiQuotePersistence(t *testing.T) {
	h := newIntegrationHarness(t)
	token := getToken(t, h.server.URL, h.clientID, h.secret, true)
	pickup, dropoff := testPoints()
	for _, payment := range []string{"cash", "partner_pay"} {
		response := requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/estimate/order",
			token, map[string]any{
				"service_ids": []any{map[string]any{"id": 2179}, map[string]any{"id": 2180}},
				"order_time":  0, "payment_method": payment, "points": []Point{pickup, dropoff},
			}, http.StatusOK)["data"].(map[string]any)
		items := response["service_estimates"].([]any)
		for index, amount := range []float64{73900, 27710} {
			info := items[index].(map[string]any)["estimate_info"].(map[string]any)
			if actual := info["estimate"].(map[string]any)["total_pay"]; actual != amount {
				t.Fatalf("%s fare = %v, want %v", payment, actual, amount)
			}
		}
		bike := items[1].(map[string]any)["estimate_info"].(map[string]any)
		created := requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/create-order",
			token, map[string]any{
				"service_id": 2180, "service_type": "RIDE-TRIP", "fee_id": bike["fee_id"],
				"order_time":     parseInt64(t, response["order_time"].(string)),
				"payment_method": payment, "points": []Point{pickup, dropoff},
			}, http.StatusOK)["data"].(map[string]any)
		orderID := created["id"].(string)
		requestJSON(t, h.server.URL, http.MethodPost, "/v1/sandbox/ride/update-status", token,
			map[string]any{"update_type": "order", "order_id": orderID, "status": "ASSIGNED"},
			http.StatusOK)
		order := requestJSON(t, h.server.URL, http.MethodGet,
			"/v1/sandbox/ride/orders/"+orderID+"?extra_components=driver,path", token, nil,
			http.StatusOK)["data"].(map[string]any)
		if !jsonEqual(order["fare"], bike) {
			t.Fatal("order did not preserve its accepted quote")
		}
		if vehicle := order["vehicle"].(map[string]any); vehicle["type"] != "motorcycle" {
			t.Fatalf("bike order was assigned wrong vehicle: %+v", vehicle)
		}
	}
}
