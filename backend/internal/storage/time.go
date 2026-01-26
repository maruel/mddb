// Provides a JSON-friendly Time type with millisecond precision.

package storage

import (
	"encoding/json"
	"math"
	"time"
)

// Time is a JSON encoded unix timestamp in 1ms units.
type Time int64

// AsTime returns the time as UTC so its string value doesn't depend on the local time zone.
func (t *Time) AsTime() time.Time {
	return time.UnixMilli(int64(*t)).UTC()
}

// ToTime converts a time.Time to a storage.Time.
func ToTime(v time.Time) Time {
	return Time(v.UnixMilli())
}

// Now returns the current time as a storage.Time.
func Now() Time {
	return ToTime(time.Now())
}

// IsZero returns true if the time is zero.
func (t Time) IsZero() bool {
	return t == 0
}

// After reports whether the time instant t is after u.
func (t Time) After(u Time) bool {
	return t > u
}

// Before reports whether the time instant t is before u.
func (t Time) Before(u Time) bool {
	return t < u
}

// MarshalJSON encodes the time as a float32 representing seconds.
func (t Time) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(t) / 1000)
}

// UnmarshalJSON decodes JSON numbers as unix timestamps, converting float64 to int64 by rounding.
func (t *Time) UnmarshalJSON(b []byte) error {
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*t = Time(math.Round(f * 1000))
	return nil
}
