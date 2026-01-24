package storage

import (
	"encoding/json"
	"math"
	"time"
)

// Time is a JSON encoded unix timestamp.
type Time int64

// AsTime returns the time as UTC so its string value doesn't depend on the local time zone.
func (t *Time) AsTime() time.Time {
	return time.Unix(int64(*t), 0).UTC()
}

// ToTime converts a time.Time to a storage.Time.
func ToTime(v time.Time) Time {
	return Time(v.Unix())
}

// UnmarshalJSON decodes JSON numbers as unix timestamps, converting float64 to int64 by rounding.
func (t *Time) UnmarshalJSON(b []byte) error {
	var i int64
	if err := json.Unmarshal(b, &i); err == nil {
		*t = Time(i)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	*t = Time(int64(math.Round(f)))
	return nil
}
