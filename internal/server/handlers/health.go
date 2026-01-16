package handlers

import "context"

// HealthRequest is the request type for health check (empty).
type HealthRequest struct{}

// HealthResponse is the response for health check.
type HealthResponse struct {
	Status string `json:"status"`
}

// Health returns the health status of the server.
func Health(ctx context.Context, req HealthRequest) (*HealthResponse, error) {
	return &HealthResponse{Status: "ok"}, nil
}
