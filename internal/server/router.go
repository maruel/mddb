package server

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/maruel/mddb/internal/server/handlers"
	"github.com/maruel/mddb/internal/storage"
)

// NewRouter creates and configures the HTTP router.
// Serves API endpoints at /api/* and static SolidJS frontend at /.
func NewRouter(fileStore *storage.FileStore) http.Handler {
	mux := &http.ServeMux{}
	ph := handlers.NewPageHandler(fileStore)
	dh := handlers.NewDatabaseHandler(fileStore)
	ah := handlers.NewAssetHandler(fileStore)

	// Health check
	mux.Handle("/api/health", Wrap(handlers.Health))

	// Pages endpoints
	mux.Handle("GET /api/pages", Wrap(ph.ListPages))
	mux.Handle("GET /api/pages/{id}", Wrap(ph.GetPage))
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

	// Assets endpoints
	mux.Handle("GET /api/assets", Wrap(ah.ListAssets))
	mux.Handle("POST /api/assets", Wrap(ah.UploadAsset))
	mux.Handle("DELETE /api/assets/{id}", Wrap(ah.DeleteAsset))
	mux.Handle("GET /assets/{id}", Wrap(ah.ServeAsset))

	// Serve static files for SolidJS frontend with SPA fallback
	publicDir := filepath.Join(fileStore.RootDir(), "public")
	mux.Handle("/", NewSPAHandler(http.Dir(publicDir)))

	return mux
}

// SPAHandler serves a single-page application, falling back to index.html for unknown routes.
type SPAHandler struct {
	fs http.FileSystem
}

// NewSPAHandler creates a new SPA handler.
func NewSPAHandler(fs http.FileSystem) *SPAHandler {
	return &SPAHandler{fs: fs}
}

// ServeHTTP implements http.Handler for SPA routing.
func (h *SPAHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try to serve the exact file
	file, err := h.fs.Open(r.URL.Path)
	if err == nil {
		if err := file.Close(); err != nil {
			_ = err
		}
		// File exists, serve it
		fs := http.FileServer(h.fs)
		// Set cache headers for static assets with extensions
		if strings.Contains(r.URL.Path, ".") {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		fs.ServeHTTP(w, r)
		return
	}

	// File not found - fall back to index.html for SPA routing
	file, err = h.fs.Open("/index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = file.Close() }()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "/index.html", time.Now(), file)
}
