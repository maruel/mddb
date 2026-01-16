package server

import (
	"net/http"
	"path/filepath"

	"github.com/maruel/mddb/internal/server/handlers"
	"github.com/maruel/mddb/internal/storage"
)

// NewRouter creates and configures the HTTP router
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
	mux.Handle("POST /api/databases/{id}/records", Wrap(dh.CreateRecord))
	mux.Handle("PUT /api/databases/{id}/records/{rid}", Wrap(dh.UpdateRecord))
	mux.Handle("DELETE /api/databases/{id}/records/{rid}", Wrap(dh.DeleteRecord))

	// Assets endpoints
	mux.Handle("GET /api/assets", Wrap(ah.ListAssets))
	mux.Handle("POST /api/assets", Wrap(ah.UploadAsset))
	mux.Handle("DELETE /api/assets/{id}", Wrap(ah.DeleteAsset))
	mux.Handle("GET /assets/{id}", Wrap(ah.ServeAsset))

	// Serve static files for SolidJS frontend
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join(fileStore.RootDir(), "public"))))

	return mux
}
