// Keeps the e2e behavior and its flag out of a production binary.

//go:build !e2e

package main

import "flag"

// e2eBuild is always false: a production binary must not fake the OAuth
// providers or raise the server user quota.
const e2eBuild = false

// fastRateLimit is always off: a production build does not define
// -fast-rate-limit, so passing it stops startup with "flag provided but not
// defined" instead of being ignored.
var fastRateLimit bool

// registerFastRateLimitFlag defines no flags on fs.
func registerFastRateLimitFlag(*flag.FlagSet) {}
