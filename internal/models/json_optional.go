package models

import (
	"bytes"
	"encoding/json"
)

// Optional represents a JSON field that may be absent or set to null.
// Set indicates whether the field was provided in the payload (even if null).
// Valid indicates whether the field contains a non-null value.
type Optional[T any] struct {
	Value T
	Set   bool
	Valid bool
}

// UnmarshalJSON captures both presence and nullability of JSON fields.
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Set = true

	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) || len(trimmed) == 0 {
		o.Valid = false
		var zero T
		o.Value = zero
		return nil
	}

	var v T
	if err := json.Unmarshal(trimmed, &v); err != nil {
		o.Valid = false
		var zero T
		o.Value = zero
		return err
	}

	o.Value = v
	o.Valid = true
	return nil
}

// Ptr returns a pointer to the value when valid, otherwise nil.
func (o Optional[T]) Ptr() *T {
	if !o.Valid {
		return nil
	}
	val := o.Value
	return &val
}
