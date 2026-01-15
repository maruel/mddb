package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/maruel/mddb/internal/storage"
	"github.com/maruel/mddb/internal/utils"
)

// PageHandler handles page-related HTTP requests
type PageHandler struct {
	fileStore *storage.FileStore
}

// NewPageHandler creates a new page handler
func NewPageHandler(fileStore *storage.FileStore) *PageHandler {
	return &PageHandler{fileStore: fileStore}
}

// ListPages returns a list of all pages
func (h *PageHandler) ListPages(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement listing pages from filesystem
	utils.RespondSuccess(w, http.StatusOK, []interface{}{})
}

// GetPage returns a specific page by ID
func (h *PageHandler) GetPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// TODO: Implement getting a page
	utils.RespondError(w, http.StatusNotFound, "Page not found", "NOT_FOUND")
	_ = id
}

// CreatePage creates a new page
func (h *PageHandler) CreatePage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondError(w, http.StatusBadRequest, "Invalid request body", "INVALID_REQUEST")
		return
	}

	// TODO: Implement creating a page
	utils.RespondSuccess(w, http.StatusCreated, map[string]string{"id": "placeholder"})
}

// UpdatePage updates an existing page
func (h *PageHandler) UpdatePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// TODO: Implement updating a page
	utils.RespondError(w, http.StatusNotFound, "Page not found", "NOT_FOUND")
	_ = id
}

// DeletePage deletes a page
func (h *PageHandler) DeletePage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// TODO: Implement deleting a page
	utils.RespondSuccess(w, http.StatusNoContent, nil)
	_ = id
}
