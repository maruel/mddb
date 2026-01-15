// Package entity defines shared domain models used across storage packages.
//
// This package contains types that are shared between the content and identity
// packages. Currently it only contains the Quota type which is used by both
// identity (for organization settings) and content (for quota enforcement).
//
// Most content-related types (Node, DataRecord, Asset, etc.) have been moved
// to the content package where they are primarily used.
package entity
