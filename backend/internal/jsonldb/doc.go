// Package jsonldb provides a generic, concurrent-safe, JSONL-backed data store.
//
// # Overview
//
// The package centers around [Table], a generic container that stores rows in a
// JSONL (JSON Lines) file with full in-memory caching for fast reads. Tables are
// safe for concurrent use by multiple goroutines.
//
// # Concurrency: Pessimistic Locking
//
// Table uses pessimistic locking: [Table.Modify] holds the write lock for the
// entire read-modify-write operation. This guarantees success without retries,
// unlike optimistic CAS which requires retry loops when concurrent writes collide.
// The tradeoff is lower throughput under high contention, but this is acceptable
// for local file storage with low concurrency.
//
// # Secondary Indexes
//
// [UniqueIndex] and [Index] provide O(1) lookups by arbitrary keys, staying
// synchronized with table mutations via [TableObserver].
//
// # File Format
//
// JSONL files with line 1 as schema header, subsequent lines as JSON rows.
// Rows are sorted by ID on load if out of order (handles clock drift, manual edits).
package jsonldb
