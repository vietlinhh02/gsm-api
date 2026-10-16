package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouteGeometryAndProviderFailures(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantError bool
	}{
		{"geometry", 200, `{"code":"Ok","routes":[{"distance":4800,"duration":600,` +
			`"geometry":{"type":"LineString","coordinates":[[105.85,21.02],[105.81,21.03]]}}]}`, false},
		{"provider error", 503, `{}`, true},
		{"no route", 200, `{"code":"NoRoute","routes":[]}`, true},
		{"malformed json", 200, `{`, true},
		{"negative distance", 200, `{"code":"Ok","routes":[{"distance":-1,"duration":60}]}`, true},
		{"negative duration", 200, `{"code":"Ok","routes":[{"distance":1,"duration":-1}]}`, true},
		{"invalid geometry", 200, `{"code":"Ok","routes":[{"distance":1,"duration":1,` +
			`"geometry":{"type":"Point","coordinates":[[105,21],[106,22]]}}]}`, true},
		{"missing geometry coordinates", 200, `{"code":"Ok","routes":[{"distance":1,"duration":1,` +
			`"geometry":{"type":"LineString","coordinates":null}}]}`, true},
		{"malformed geometry coordinates", 200, `{"code":"Ok","routes":[{"distance":1,"duration":1,` +
			`"geometry":{"type":"LineString","coordinates":[[105],[106,22]]}}]}`, true},
		{"out of bounds geometry", 200, `{"code":"Ok","routes":[{"distance":1,"duration":1,` +
			`"geometry":{"type":"LineString","coordinates":[[105,91],[106,22]]}}]}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("geometries") != "geojson" {
					t.Error("route must request GeoJSON")
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer provider.Close()
			osm := NewOSM(provider.URL, provider.URL, provider.Client())
			pickup, dropoff := testPoints()
			route, err := osm.route(context.Background(), []Point{pickup, dropoff})
			if (err != nil) != tc.wantError {
				t.Fatalf("route error = %v", err)
			}
			if !tc.wantError && (route.Distance != 4800 || len(route.Geometry.Coordinates) != 2) {
				t.Fatalf("unexpected route: %+v", route)
			}
		})
	}
}

func TestSimulatedVehicleMatchesTravelMode(t *testing.T) {
	if vehicle := simulatedVehicle("Motorcycle"); vehicle["type"] != "motorcycle" {
		t.Fatalf("motorcycle order assigned wrong vehicle: %+v", vehicle)
	}
	if vehicle := simulatedVehicle("Car"); vehicle["type"] != "car" {
		t.Fatalf("car order assigned wrong vehicle: %+v", vehicle)
	}
}
