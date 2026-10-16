package api

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProductionRoutesAreNotExposed(t *testing.T) {
	handler := (&API{Logger: log.New(io.Discard, "", 0)}).Handler()
	for _, route := range []struct{ method, path string }{
		{"POST", "/v1/public/token"},
		{"POST", "/v1/ride/list-available-services"},
		{"POST", "/v1/ride/estimate/order"},
		{"POST", "/v1/ride/estimate/hour-order"},
		{"POST", "/v1/ride/create-order"},
		{"GET", "/v1/ride/orders/test"},
		{"GET", "/v1/ride/list-user-orders"},
		{"POST", "/v1/ride/cancel-order"},
		{"GET", "/v1/place/geocode"},
	} {
		t.Run(route.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
			if response.Code != http.StatusNotFound {
				t.Fatalf("removed route returned %d, want 404", response.Code)
			}
		})
	}
}
