package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type photonFeature struct {
	Geometry struct {
		Coordinates []float64 `json:"coordinates"`
	} `json:"geometry"`
	Properties struct {
		OSMType     string `json:"osm_type"`
		OSMID       int64  `json:"osm_id"`
		OSMKey      string `json:"osm_key"`
		OSMValue    string `json:"osm_value"`
		Name        string `json:"name"`
		Street      string `json:"street"`
		HouseNumber string `json:"housenumber"`
		District    string `json:"district"`
		City        string `json:"city"`
		State       string `json:"state"`
		Country     string `json:"country"`
	} `json:"properties"`
}

func photonQuery(input geocodeInput) (string, url.Values) {
	query := url.Values{"limit": {strconv.Itoa(input.Max)}}
	path := "/api/"
	if input.Lat != nil && input.Lng != nil {
		path = "/reverse"
		query.Set("lat", strconv.FormatFloat(*input.Lat, 'f', 8, 64))
		query.Set("lon", strconv.FormatFloat(*input.Lng, 'f', 8, 64))
	} else {
		query.Set("q", input.Text)
		query.Set("countrycode", "VN")
	}
	return path, query
}

func (o *OSM) fetchPhoton(ctx context.Context, input geocodeInput) ([]photonFeature, error) {
	path, query := photonQuery(input)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet,
		o.Photon+path+"?"+query.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("create Photon request: %w", err)
	}
	request.Header.Set("User-Agent", "GSM-Rides-API-Simulator/1.0 (local development)")
	response, err := o.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request Photon geocoding: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Photon returned HTTP %d", response.StatusCode)
	}
	var body struct {
		Features []photonFeature `json:"features"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode Photon response: %w", err)
	}
	return body.Features, nil
}

func (o *OSM) photonGeocode(ctx context.Context, input geocodeInput) ([]GeoResult, error) {
	features, err := o.fetchPhoton(ctx, input)
	if err != nil {
		return nil, err
	}
	results := make([]GeoResult, 0, len(features))
	for _, feature := range features {
		place, err := photonPlace(feature, input.Language)
		if err != nil {
			return nil, err
		}
		if input.Lat != nil && input.Lng != nil {
			place.Distance = haversine(*input.Lat, *input.Lng, place.Latitude, place.Longitude)
		}
		results = append(results, place)
	}
	return results, nil
}

func photonPlace(feature photonFeature, language string) (GeoResult, error) {
	coordinates := feature.Geometry.Coordinates
	if len(coordinates) != 2 {
		return GeoResult{}, fmt.Errorf("Photon returned malformed coordinates")
	}
	if !validCoordinate(coordinates[0], -180, 180) || !validCoordinate(coordinates[1], -90, 90) {
		return GeoResult{}, fmt.Errorf("Photon returned coordinates outside latitude/longitude bounds")
	}
	properties := feature.Properties
	parts := []string{}
	seen := map[string]bool{}
	for _, part := range []string{
		properties.Name, strings.TrimSpace(properties.HouseNumber + " " + properties.Street),
		properties.District, properties.City, properties.State, properties.Country,
	} {
		if part != "" && !seen[part] {
			parts = append(parts, part)
			seen[part] = true
		}
	}
	place, err := mapPlace(nominatimPlace{
		OSMType: properties.OSMType, OSMID: properties.OSMID,
		Lat: strconv.FormatFloat(coordinates[1], 'f', 8, 64),
		Lon: strconv.FormatFloat(coordinates[0], 'f', 8, 64), DisplayName: strings.Join(parts, ", "),
		Category: properties.OSMKey, Type: properties.OSMValue,
		Address: map[string]string{
			"road": properties.Street, "city_district": properties.District,
			"city": properties.City, "state": properties.State,
		},
	}, language)
	if err != nil {
		return GeoResult{}, err
	}
	place.PlaceTypeOSM["provider"] = "Photon"
	return place, nil
}
