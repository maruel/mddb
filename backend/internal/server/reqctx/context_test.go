// Tests for request context utilities.

package reqctx

import (
	"strings"
	"testing"
)

func TestRequestID(t *testing.T) {
	if got := RequestID(t.Context()); got != "" {
		t.Fatalf("missing hint: got %q", got)
	}
	for _, id := range []string{"write-one", strings.Repeat("a", 64)} {
		if got := RequestID(WithRequestID(t.Context(), id)); got != id {
			t.Errorf("hint %q: got %q", id, got)
		}
	}
	if got := RequestID(WithRequestID(t.Context(), strings.Repeat("a", 65))); got != "" {
		t.Fatalf("oversized hint: got %q", got)
	}
}
