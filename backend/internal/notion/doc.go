// Package notion provides a client and extractor for the Notion API.
//
// This package enables importing Notion workspaces into mddb, handling:
//   - API client with rate limiting (3 req/sec)
//   - Extraction of pages, databases, and records
//   - Type mapping from Notion properties to mddb properties
//   - Progress reporting for CLI and API consumers
package notion
