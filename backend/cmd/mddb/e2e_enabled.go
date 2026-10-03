// Selects the e2e behavior: the fake providers and the raised limits.

//go:build e2e

package main

// e2eBuild reports whether this binary was built with the e2e build tag. It
// selects the fake OAuth providers, the raised server user quota, and the
// 10000x rate-limit multiplier.
const e2eBuild = true
