package server

import (
	"net/http"
	"path/filepath"

	"github.com/maruel/mddb/internal/server/handlers"
	"github.com/maruel/mddb/internal/storage"
)

// NewRouter creates and configures the HTTP router
func NewRouter(fileStore *storage.FileStore) http.Handler {
	mux := http.NewServeMux()

	// Initialize handlers
	pageHandler := handlers.NewPageHandler(fileStore)
	databaseHandler := handlers.NewDatabaseHandler(fileStore)
	assetHandler := handlers.NewAssetHandler(fileStore)
	healthHandler := handlers.NewHealthHandler()

	// Health check
	mux.HandleFunc("/api/health", healthHandler.Health)

	// Pages endpoints
	mux.HandleFunc("GET /api/pages", pageHandler.ListPages)
	mux.HandleFunc("GET /api/pages/{id}", pageHandler.GetPage)
	mux.HandleFunc("POST /api/pages", pageHandler.CreatePage)
	mux.HandleFunc("PUT /api/pages/{id}", pageHandler.UpdatePage)
	mux.HandleFunc("DELETE /api/pages/{id}", pageHandler.DeletePage)

	// Database endpoints
	mux.HandleFunc("GET /api/databases", databaseHandler.ListDatabases)
	mux.HandleFunc("GET /api/databases/{id}", databaseHandler.GetDatabase)
	mux.HandleFunc("POST /api/databases", databaseHandler.CreateDatabase)
	mux.HandleFunc("PUT /api/databases/{id}", databaseHandler.UpdateDatabase)
	mux.HandleFunc("DELETE /api/databases/{id}", databaseHandler.DeleteDatabase)

	// Records endpoints
	mux.HandleFunc("GET /api/databases/{id}/records", databaseHandler.ListRecords)
	mux.HandleFunc("POST /api/databases/{id}/records", databaseHandler.CreateRecord)
	mux.HandleFunc("PUT /api/databases/{id}/records/{rid}", databaseHandler.UpdateRecord)
	mux.HandleFunc("DELETE /api/databases/{id}/records/{rid}", databaseHandler.DeleteRecord)

	// Assets endpoints
	mux.HandleFunc("GET /api/assets", assetHandler.ListAssets)
	mux.HandleFunc("POST /api/assets", assetHandler.UploadAsset)
	mux.HandleFunc("DELETE /api/assets/{id}", assetHandler.DeleteAsset)
	mux.HandleFunc("GET /assets/{id}", assetHandler.ServeAsset)

	// Serve static files for SolidJS frontend
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join(fileStore.RootDir(), "public"))))

	return mux
}
