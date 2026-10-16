package api

import (
	"encoding/json"
	"fmt"
)

type jsonScanner struct {
	target any
}

func (s jsonScanner) Scan(source any) error {
	value, ok := source.([]byte)
	if !ok {
		return fmt.Errorf("expected JSON bytes, got %T", source)
	}
	if err := json.Unmarshal(value, s.target); err != nil {
		return fmt.Errorf("decode JSON database value: %w", err)
	}
	return nil
}
