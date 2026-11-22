// Tests the fixed-width timestamp encoding and decoding of earlier encodings.

package v1

import (
	"encoding/json"
	"math"
	"testing"
)

func TestTime(t *testing.T) {
	t.Run("MarshalJSON", func(t *testing.T) {
		for _, tc := range []struct {
			in   Time
			want string
		}{
			{0, "0.000"},
			{999, "0.999"},
			{1_763_844_580_100, "1763844580.100"},
			{1_763_844_580_123, "1763844580.123"},
			{-500, "-0.500"},
			{-1_500, "-1.500"},
			{math.MinInt64, "-9223372036854775.808"},
		} {
			got, err := json.Marshal(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("%d: got %s, want %s", tc.in, got, tc.want)
			}
			var back Time
			if err := json.Unmarshal(got, &back); err != nil {
				t.Fatal(err)
			}
			// float64 decoding is exact within 2^53 milliseconds.
			if tc.in != math.MinInt64 && back != tc.in {
				t.Errorf("%s: decoded %d, want %d", got, back, tc.in)
			}
		}
	})
	t.Run("UnmarshalJSON", func(t *testing.T) {
		for _, tc := range []struct {
			in   string
			want Time
		}{
			{"1763844580.1", 1_763_844_580_100},
			{"1763844580", 1_763_844_580_000},
			{"1763844580.123", 1_763_844_580_123},
		} {
			var got Time
			if err := json.Unmarshal([]byte(tc.in), &got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("%s: got %d, want %d", tc.in, got, tc.want)
			}
		}
	})
}
