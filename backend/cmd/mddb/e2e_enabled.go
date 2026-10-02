// Selects the e2e behavior: the raised quota, fake providers, and the flag.

//go:build e2e

package main

import "flag"

// e2eBuild reports whether this binary was built with the e2e build tag. It
// selects the fake OAuth providers and the raised server user quota.
const e2eBuild = true

// fastRateLimit asks the server to multiply every rate limit 10000x. It is set
// by the -fast-rate-limit flag, which only an e2e build defines.
var fastRateLimit bool

// registerFastRateLimitFlag defines -fast-rate-limit on fs. It defaults to off,
// so an e2e server started without the flag keeps production rate limits.
func registerFastRateLimitFlag(fs *flag.FlagSet) {
	fs.BoolVar(&fastRateLimit, "fast-rate-limit", false, "Multiply every rate limit 10000x (needed by the parallel e2e suite)")
}
