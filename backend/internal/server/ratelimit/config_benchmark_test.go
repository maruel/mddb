// Benchmarks authenticated route classification for read-only MCP POST requests.

package ratelimit

import "testing"

var benchmarkMatchAuthTier *Tier
var benchmarkMatchAuthPath = "/api/v1/gomode/mcp"

func BenchmarkMatchAuthMCP(b *testing.B) {
	limiters := NewLimiters(ConfigFromStorage(5, 60, 30000, 6000, false))
	b.ReportAllocs()
	for b.Loop() {
		benchmarkMatchAuthTier = limiters.MatchAuth("POST", benchmarkMatchAuthPath)
	}
}
