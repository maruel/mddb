// Keeps the e2e behavior out of a production binary.

//go:build !e2e

package main

// e2eBuild is always false: a production binary must not fake the OAuth
// providers, raise the server user quota, or multiply the rate limits.
const e2eBuild = false
