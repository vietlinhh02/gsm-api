package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (a *API) geocode(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	text := strings.TrimSpace(query.Get("text"))
	reverse := query.Get("reverse") == "true"
	if !reverse && query.Has("reverse") && query.Get("reverse") != "false" {
		writeError(w, http.StatusBadRequest, 400, "reverse must be true or omitted")
		return
	}
	var lat, lng *float64
	if reverse {
		var err error
		lat, err = parseCoordinate(query.Get("lat"), -90, 90)
		if err != nil {
			writeError(w, http.StatusBadRequest, 400, "reverse geocode requires valid lat")
			return
		}
		lng, err = parseCoordinate(query.Get("lng"), -180, 180)
		if err != nil {
			writeError(w, http.StatusBadRequest, 400, "reverse geocode requires valid lng")
			return
		}
	} else if text == "" {
		writeError(w, http.StatusBadRequest, 400, "geocode requires text")
		return
	}
	max := 5
	if query.Get("max") != "" {
		parsed, err := strconv.Atoi(query.Get("max"))
		if err != nil || parsed < 1 || parsed > 40 {
			writeError(w, http.StatusBadRequest, 400, "max must be an integer from 1 to 40")
			return
		}
		max = parsed
	}
	language := r.Header.Get("Accept-Language")
	if language == "" {
		language = "vi"
	}
	cacheKey := geocodeCacheKey(text, lat, lng, max, language)
	var cached []GeoResult
	err := a.Store.DB.QueryRowContext(r.Context(), `
		SELECT response FROM geocode_cache
		WHERE cache_key=$1 AND expires_at>NOW()
	`, cacheKey).
		Scan(jsonScanner{target: &cached})
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"data": cached})
		return
	}
	if err != nil && err != sql.ErrNoRows {
		a.dbError(w, "read geocode cache", err)
		return
	}
	results, err := a.OSM.geocode(r.Context(), geocodeInput{
		Text: text, Lat: lat, Lng: lng, Max: max, Language: language,
	})
	if err != nil {
		a.Logger.Printf("OSM geocode: %v", err)
		writeError(w, http.StatusBadGateway, 502, "geocoding provider is unavailable")
		return
	}
	if len(results) == 0 {
		writeError(w, http.StatusNotFound, 404, "no results found")
		return
	}
	encoded, err := json.Marshal(results)
	if err != nil {
		a.dbError(w, "encode geocode cache", err)
		return
	}
	_, err = a.Store.DB.ExecContext(
		r.Context(),
		`INSERT INTO geocode_cache(cache_key,response,expires_at)
		 VALUES($1,$2,NOW()+INTERVAL '30 days')
		 ON CONFLICT(cache_key) DO UPDATE SET
		 response=EXCLUDED.response,expires_at=EXCLUDED.expires_at`,
		cacheKey,
		encoded,
	)
	if err != nil {
		a.Logger.Printf("write geocode cache: %v", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": results})
}

func parseCoordinate(value string, minimum, maximum float64) (*float64, error) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || !validCoordinate(parsed, minimum, maximum) {
		return nil, fmt.Errorf("coordinate outside valid range")
	}
	return &parsed, nil
}
