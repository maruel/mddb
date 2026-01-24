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
// # Blob Storage
//
// Row types can include [Blob] fields for large binary data. Blobs are stored as
// content-addressed files in a sibling directory (mytable.jsonl → mytable.blobs/),
// with only the reference stored in the JSONL row. Use [Table.NewBlob] to create
// blobs via streaming writes, then assign the returned [Blob] to row fields.
// Blob files are automatically deduplicated by content hash and garbage collected
// when no longer referenced.
//
// # File Format
//
// JSONL files with line 1 as schema header, subsequent lines as JSON rows.
// Rows are sorted by ID on load if out of order (handles clock drift, manual edits).
// Blob references use the format "sha256:<BASE32>-<size>" for self-describing,
// content-addressed storage with compact encoding.
package jsonldb
