// Package server implements the HTTP server and routing logic.
package server

import (
	"embed"
	"io"
	"io/fs"
	"net/http"

	"github.com/maruel/mddb/frontend"
	"github.com/maruel/mddb/internal/server/handlers"
	"github.com/maruel/mddb/internal/storage"
)

// NewRouter creates and configures the HTTP router.
// Serves API endpoints at /api/* and static SolidJS frontend at /.
func NewRouter(fileStore *storage.FileStore, gitService *storage.GitService) http.Handler {
	mux := &http.ServeMux{}
	ph := handlers.NewPageHandler(fileStore, gitService)
	dh := handlers.NewDatabaseHandler(fileStore, gitService)
	ah := handlers.NewAssetHandler(fileStore, gitService)
	sh := handlers.NewSearchHandler(fileStore)

	// Health check
	mux.Handle("/api/health", Wrap(handlers.Health))

	// Pages endpoints
	mux.Handle("GET /api/pages", Wrap(ph.ListPages))
	mux.Handle("GET /api/pages/{id}", Wrap(ph.GetPage))
	mux.Handle("GET /api/pages/{id}/history", Wrap(ph.GetPageHistory))
	mux.Handle("GET /api/pages/{id}/history/{hash}", Wrap(ph.GetPageVersion))
	mux.Handle("POST /api/pages", Wrap(ph.CreatePage))
	mux.Handle("PUT /api/pages/{id}", Wrap(ph.UpdatePage))
	mux.Handle("DELETE /api/pages/{id}", Wrap(ph.DeletePage))

	// Database endpoints
	mux.Handle("GET /api/databases", Wrap(dh.ListDatabases))
	mux.Handle("GET /api/databases/{id}", Wrap(dh.GetDatabase))
	mux.Handle("POST /api/databases", Wrap(dh.CreateDatabase))
	mux.Handle("PUT /api/databases/{id}", Wrap(dh.UpdateDatabase))
	mux.Handle("DELETE /api/databases/{id}", Wrap(dh.DeleteDatabase))

	// Records endpoints
	mux.Handle("GET /api/databases/{id}/records", Wrap(dh.ListRecords))
	mux.Handle("GET /api/databases/{id}/records/{rid}", Wrap(dh.GetRecord))
	mux.Handle("POST /api/databases/{id}/records", Wrap(dh.CreateRecord))
	mux.Handle("PUT /api/databases/{id}/records/{rid}", Wrap(dh.UpdateRecord))
	mux.Handle("DELETE /api/databases/{id}/records/{rid}", Wrap(dh.DeleteRecord))

	// Assets endpoints (page-based)
	mux.Handle("GET /api/pages/{id}/assets", Wrap(ah.ListPageAssets))
	mux.HandleFunc("POST /api/pages/{id}/assets", ah.UploadPageAssetHandler)
	mux.Handle("DELETE /api/pages/{id}/assets/{name}", Wrap(ah.DeletePageAsset))

	// Search endpoint
	mux.Handle("POST /api/search", Wrap(sh.Search))

	// File serving (raw asset files)
	mux.HandleFunc("GET /assets/{id}/{name}", ah.ServeAssetFile)

	// Serve embedded SolidJS frontend with SPA fallback
	mux.Handle("/", NewEmbeddedSPAHandler(frontend.Files))

	return mux
}

// EmbeddedSPAHandler serves an embedded single-page application with fallback to index.html.
type EmbeddedSPAHandler struct {
	fs embed.FS
}

// NewEmbeddedSPAHandler creates a handler for the embedded frontend.
func NewEmbeddedSPAHandler(fs embed.FS) *EmbeddedSPAHandler {
	return &EmbeddedSPAHandler{fs: fs}
}

// ServeHTTP implements http.Handler for embedded SPA routing.
func (h *EmbeddedSPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try to serve the exact file from dist/
	path := "dist" + r.URL.Path
	f, err := h.fs.Open(path)
	if err == nil {
		_ = f.Close()
		// File exists, serve it from embedded FS
		fsys, _ := fs.Sub(h.fs, "dist")
		fileServer := http.FileServer(http.FS(fsys))
		// Set cache headers for static assets with extensions
		if containsDot(r.URL.Path) {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		fileServer.ServeHTTP(w, r)
		return
	}

	// File not found - fall back to index.html for SPA routing
	indexFile, err := h.fs.Open("dist/index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = indexFile.Close() }()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	// Serve index.html
	_, _ = io.Copy(w, indexFile)
}

// containsDot checks if a path contains a dot (file extension).
func containsDot(path string) bool {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return false
		}
		if path[i] == '.' {
			return true
		}
	}
	return false
}
