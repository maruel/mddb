// Tests for API request validation logic.

package dto

import (
	"testing"

	"github.com/maruel/ksid"
)

func TestListRecordsRequest_Validate(t *testing.T) {
	wsID := ksid.NewID()

	t.Run("missing wsID", func(t *testing.T) {
		req := &ListRecordsRequest{}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for missing wsID")
		}
	})
	t.Run("defaults limit when zero", func(t *testing.T) {
		req := &ListRecordsRequest{WsID: wsID}
		if err := req.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Limit != DefaultPageLimit {
			t.Errorf("Limit = %d, want %d", req.Limit, DefaultPageLimit)
		}
	})
	t.Run("rejects negative offset", func(t *testing.T) {
		req := &ListRecordsRequest{WsID: wsID, Offset: -5, Limit: 10}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for negative offset")
		}
	})
	t.Run("rejects negative limit", func(t *testing.T) {
		req := &ListRecordsRequest{WsID: wsID, Limit: -1}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for negative limit")
		}
	})
	t.Run("clamps excessive limit", func(t *testing.T) {
		req := &ListRecordsRequest{WsID: wsID, Limit: 9999}
		if err := req.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Limit != MaxPageLimit {
			t.Errorf("Limit = %d, want %d", req.Limit, MaxPageLimit)
		}
	})
	t.Run("preserves valid values", func(t *testing.T) {
		req := &ListRecordsRequest{WsID: wsID, Offset: 20, Limit: 50}
		if err := req.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Offset != 20 {
			t.Errorf("Offset = %d, want 20", req.Offset)
		}
		if req.Limit != 50 {
			t.Errorf("Limit = %d, want 50", req.Limit)
		}
	})
}

func TestListNotificationsRequest_Validate(t *testing.T) {
	t.Run("defaults limit when zero", func(t *testing.T) {
		req := &ListNotificationsRequest{}
		if err := req.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Limit != DefaultPageLimit {
			t.Errorf("Limit = %d, want %d", req.Limit, DefaultPageLimit)
		}
	})
	t.Run("rejects negative offset", func(t *testing.T) {
		req := &ListNotificationsRequest{Offset: -1, Limit: 10}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for negative offset")
		}
	})
	t.Run("rejects negative limit", func(t *testing.T) {
		req := &ListNotificationsRequest{Limit: -1}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for negative limit")
		}
	})
	t.Run("clamps excessive limit", func(t *testing.T) {
		req := &ListNotificationsRequest{Limit: 5000}
		if err := req.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Limit != MaxPageLimit {
			t.Errorf("Limit = %d, want %d", req.Limit, MaxPageLimit)
		}
	})
}

func TestListNodeVersionsRequest_Validate(t *testing.T) {
	wsID := ksid.NewID()

	t.Run("missing wsID", func(t *testing.T) {
		req := &ListNodeVersionsRequest{}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for missing wsID")
		}
	})
	t.Run("defaults limit when zero", func(t *testing.T) {
		req := &ListNodeVersionsRequest{WsID: wsID}
		if err := req.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Limit != MaxVersionsLimit {
			t.Errorf("Limit = %d, want %d", req.Limit, MaxVersionsLimit)
		}
	})
	t.Run("rejects negative limit", func(t *testing.T) {
		req := &ListNodeVersionsRequest{WsID: wsID, Limit: -1}
		if err := req.Validate(); err == nil {
			t.Fatal("expected error for negative limit")
		}
	})
	t.Run("clamps excessive limit", func(t *testing.T) {
		req := &ListNodeVersionsRequest{WsID: wsID, Limit: 5000}
		if err := req.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Limit != MaxVersionsLimit {
			t.Errorf("Limit = %d, want %d", req.Limit, MaxVersionsLimit)
		}
	})
	t.Run("preserves valid limit", func(t *testing.T) {
		req := &ListNodeVersionsRequest{WsID: wsID, Limit: 100}
		if err := req.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if req.Limit != 100 {
			t.Errorf("Limit = %d, want 100", req.Limit)
		}
	})
}
