// Tests the lifecycle of background work owned by handler services.

package handlers

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServices(t *testing.T) {
	t.Run("StopBackground", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			var svc Services
			started := make(chan struct{})
			finished := make(chan struct{})
			if !svc.StartBackground(t.Context(), time.Minute, func(ctx context.Context) {
				close(started)
				<-ctx.Done()
				close(finished)
			}) {
				t.Fatal("work rejected before shutdown")
			}
			<-started
			if err := svc.StopBackground(t.Context()); err != nil {
				t.Fatal(err)
			}
			select {
			case <-finished:
			default:
				t.Fatal("shutdown returned before work finished")
			}
			if svc.StartBackground(t.Context(), time.Minute, func(context.Context) {}) {
				t.Fatal("work accepted after shutdown")
			}
		})
		t.Run("error", func(t *testing.T) {
			var svc Services
			release := make(chan struct{})
			if !svc.StartBackground(t.Context(), time.Minute, func(context.Context) { <-release }) {
				t.Fatal("work rejected before shutdown")
			}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if err := svc.StopBackground(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("StopBackground: got %v, want context.Canceled", err)
			}
			close(release)
			if err := svc.StopBackground(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	})
}
