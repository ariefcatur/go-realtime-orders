package kafka

import (
	"encoding/json"
	"fmt"
)

func MustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func UnmarshalEnvelope(b []byte, out any) error {
	return json.Unmarshal(b, out)
}

// Unwrap memudahkan decode payload spesifik
func UnwrapPayload[T any](payload json.RawMessage) (T, error) {
	var t T
	if err := json.Unmarshal(payload, &t); err != nil {
		return t, fmt.Errorf("decode payload: %w", err)
	}
	return t, nil
}
