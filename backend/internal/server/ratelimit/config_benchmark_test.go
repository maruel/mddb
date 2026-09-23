// Benchmarks authenticated route classification for read-only MCP POST requests.

package ratelimit

import "testing"

var benchmarkMatchAuthTier *Tier
var benchmarkMatchAuthPath = "/api/v1/gomode/mcp"

func BenchmarkMatchAuthMCP(b *testing.B) {
	limiters := NewLimiters(DefaultConfig())
	b.Cleanup(limiters.Close)
	b.ReportAllocs()
	for b.Loop() {
		benchmarkMatchAuthTier = limiters.MatchAuth("POST", benchmarkMatchAuthPath)
	}
}
