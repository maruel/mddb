// Benchmarks authenticated route classification for read-only MCP POST requests.

package ratelimit

import (
	"testing"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
)

var benchmarkMatchAuthTier *Tier
var benchmarkMatchAuthPath = "/api/v1/gomode/mcp"

func BenchmarkMatchAuthMCP(b *testing.B) {
	limiters := NewLimiters(storagev1.RateLimits{AuthRatePerMin: 5, WriteRatePerMin: 60, ReadAuthRatePerMin: 30000, ReadUnauthRatePerMin: 6000}, false)
	b.ReportAllocs()
	for b.Loop() {
		benchmarkMatchAuthTier = limiters.MatchAuth("POST", benchmarkMatchAuthPath)
	}
}
