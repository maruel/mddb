// Tests workspace quota transport errors and measures staged HTTP upload memory.

package server

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	storagev1 "github.com/maruel/mddb/backend/internal/storage/data/v1"
	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto/v1"
)

func quotaWorkspace(t testing.TB) (*testEnv, string, dto.WorkspaceResponse, dto.CreatePageResponse) {
	e := setupTestEnv(t)
	var login dto.AuthResponse
	if code := e.doJSON(t, "POST", "/api/v1/auth/register", dto.RegisterRequest{Email: "quota@example.com", Password: "password123", Name: "Quota"}, &login, ""); code != 200 {
		t.Fatal(code)
	}
	var org dto.OrganizationResponse
	if code := e.doJSON(t, "POST", "/api/v1/organizations", dto.CreateOrganizationRequest{Name: "Quota"}, &org, login.Token); code != 200 {
		t.Fatal(code)
	}
	var ws dto.WorkspaceResponse
	if code := e.doJSON(t, "POST", "/api/v1/organizations/"+org.ID.String()+"/workspaces", dto.CreateWorkspaceRequest{Name: "Quota"}, &ws, login.Token); code != 200 {
		t.Fatal(code)
	}
	var page dto.CreatePageResponse
	if code := e.doJSON(t, "POST", "/api/v1/workspaces/"+ws.ID.String()+"/nodes/0/page/create", dto.CreatePageRequest{Title: "Quota"}, &page, login.Token); code != 200 {
		t.Fatal(code)
	}
	return e, login.Token, ws, page
}

func uploadQuotaAsset(t testing.TB, e *testEnv, token, path, contentType string, body io.Reader, size int64) int {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, e.server.URL+path, body)
	if err != nil {
		t.Fatal(err)
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := e.server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode
}

func TestWorkspaceQuotas(t *testing.T) {
	t.Run("conditional table mutations preserve quota recovery and conflicts", func(t *testing.T) {
		e, token, ws, _ := quotaWorkspace(t)
		base := "/api/v1/workspaces/" + ws.ID.String() + "/nodes/"
		var created dto.CreateTableUnderParentResponse
		if status := e.doJSON(t, http.MethodPost, base+"0/table/create", dto.CreateTableRequest{Title: "Original table with plenty of space to shrink for serialization changes", Properties: []dto.Property{{Name: "key", Type: "text"}}}, &created, token); status != http.StatusOK {
			t.Fatal(status)
		}
		path := base + created.ID.String()
		var original dto.NodeResponse
		if status := e.doJSON(t, http.MethodGet, path, nil, &original, token); status != http.StatusOK {
			t.Fatal(status)
		}
		if _, err := e.wsService.Modify(ws.ID, func(w *identityv1.Workspace) error {
			w.Quotas.MaxStorageBytes = 0
			w.Quotas.MaxColumnsPerTable = 0
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		e.fileStore.InvalidateWorkspaceStore(ws.ID)
		change := dto.UpdateTableRequest{Title: "Short", Properties: original.Properties, ExpectedTitle: original.Title, ExpectedSchemaRevision: original.SchemaRevision}
		var saved dto.UpdateTableResponse
		if status := e.doJSON(t, http.MethodPost, path+"/table", change, &saved, token); status != http.StatusOK {
			t.Fatalf("shrinking conditional table denied: %d properties=%+v", status, original.Properties)
		}
		if status := e.doJSON(t, http.MethodPost, path+"/table", change, nil, token); status != http.StatusConflict {
			t.Fatalf("stale conditional table lost conflict: %d", status)
		}
		change.ExpectedTitle, change.ExpectedSchemaRevision = saved.Title, saved.SchemaRevision
		change.Title = saved.Title
		if status := e.doJSON(t, http.MethodPost, path+"/table", change, nil, token); status != http.StatusOK {
			t.Fatalf("unchanged conditional table denied: %d", status)
		}
		change.Properties = append(change.Properties, dto.Property{Name: "second", Type: "text"})
		if status := e.doJSON(t, http.MethodPost, path+"/table", change, nil, token); status != http.StatusTooManyRequests {
			t.Fatalf("conditional schema growth escaped admission: %d", status)
		}
		var current dto.NodeResponse
		if status := e.doJSON(t, http.MethodGet, path, nil, &current, token); status != http.StatusOK || current.Title != saved.Title || current.SchemaRevision != saved.SchemaRevision {
			t.Fatalf("rejected writes changed conditional table: status=%d node=%+v", status, current)
		}
	})
	t.Run("regular blobs-suffix replacements retain pooled recovery", func(t *testing.T) {
		for _, name := range []string{"example.blobs", "photo.bin", ".user-note"} {
			t.Run(name, func(t *testing.T) {
				e, token, ws, page := quotaWorkspace(t)
				path := "/api/v1/workspaces/" + ws.ID.String() + "/nodes/" + page.ID.String() + "/assets"
				upload := func(data string) int {
					var body bytes.Buffer
					w := multipart.NewWriter(&body)
					p, err := w.CreateFormFile("file", name)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := io.WriteString(p, data); err != nil {
						t.Fatal(err)
					}
					if err := w.Close(); err != nil {
						t.Fatal(err)
					}
					return uploadQuotaAsset(t, e, token, path, w.FormDataContentType(), &body, int64(body.Len()))
				}
				if code := upload("abcd"); code != 201 {
					t.Fatal(code)
				}
				if err := e.config.Update(t.TempDir(), func(c *storagev1.ServerConfig) error { c.Quotas.MaxTotalStorageBytes = 1; return nil }, nil); err != nil {
					t.Fatal(err)
				}
				for _, data := range []string{"efgh", "abc"} {
					if code := upload(data); code != 201 {
						t.Fatalf("replacement %q/%q: %d", name, data, code)
					}
				}
				if code := upload("new growth"); code != 429 {
					t.Fatalf("pooled growth unexpectedly admitted: %d", code)
				}
			})
		}
	})
	t.Run("frontmatter quota errors preserve their cause", func(t *testing.T) {
		e, token, ws, page := quotaWorkspace(t)
		if _, err := e.wsService.Modify(ws.ID, func(w *identityv1.Workspace) error { w.Quotas.MaxStorageBytes = 0; return nil }); err != nil {
			t.Fatal(err)
		}
		e.fileStore.InvalidateWorkspaceStore(ws.ID)
		var failure dto.ErrorResponse
		path := "/api/v1/workspaces/" + ws.ID.String() + "/nodes/" + page.ID.String() + "/page/frontmatter"
		if code := e.doJSON(t, "POST", path, dto.UpdatePageFrontmatterRequest{Icon: "icon"}, &failure, token); code != 429 || failure.Error.Code != dto.ErrorCodeQuotaExceeded {
			t.Fatalf("frontmatter response %d: %+v", code, failure)
		}
		missing := "/api/v1/workspaces/" + ws.ID.String() + "/nodes/" + ksid.NewID().String() + "/page/frontmatter"
		if code := e.doJSON(t, "POST", missing, dto.UpdatePageFrontmatterRequest{Icon: "icon"}, nil, token); code != 404 {
			t.Fatalf("missing page status: %d", code)
		}
		file := filepath.Join(e.dataDir, ws.ID.String(), page.ID.String(), "index.md")
		if err := os.Rename(file, file+".saved"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(file, 0o700); err != nil {
			t.Fatal(err)
		}
		if code := e.doJSON(t, "POST", path, dto.UpdatePageFrontmatterRequest{Icon: "icon"}, nil, token); code != 500 {
			t.Fatalf("I/O error concealed as absence: %d", code)
		}
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(file+".saved", file); err != nil {
			t.Fatal(err)
		}
	})
}

type quotaZeroReader struct{}

func (quotaZeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func BenchmarkWorkspaceUploads(b *testing.B) {
	for _, sample := range []struct {
		size, maxAsset int64
		status         int
	}{{16 << 20, 1, 429}, {64 << 20, 1, 429}, {64 << 20, 128 << 20, 201}} {
		size := sample.size
		b.Run(fmt.Sprintf("%dMiB/status%d", size>>20, sample.status), func(b *testing.B) {
			e, token, ws, page := quotaWorkspace(b)
			if err := e.config.Update(b.TempDir(), func(c *storagev1.ServerConfig) error {
				c.Quotas.MaxRequestBodyBytes = 0
				c.Quotas.MaxAssetSizeBytes = sample.maxAsset
				return nil
			}, nil); err != nil {
				b.Fatal(err)
			}
			if _, err := e.wsService.Modify(ws.ID, func(w *identityv1.Workspace) error { w.Quotas.MaxAssetSizeBytes = sample.maxAsset; return nil }); err != nil {
				b.Fatal(err)
			}
			e.fileStore.InvalidateWorkspaceStore(ws.ID)
			bodyPath := filepath.Join(b.TempDir(), "body")
			body, err := os.Create(bodyPath) //nolint:gosec // G304: Benchmark-owned temporary path.
			if err != nil {
				b.Fatal(err)
			}
			w := multipart.NewWriter(body)
			part, err := w.CreateFormFile("file", "large.bin")
			if err != nil {
				b.Fatal(err)
			}
			if _, err := io.CopyN(part, quotaZeroReader{}, size); err != nil {
				b.Fatal(err)
			}
			if err := w.Close(); err != nil {
				b.Fatal(err)
			}
			if err := body.Close(); err != nil {
				b.Fatal(err)
			}
			info, err := os.Stat(bodyPath)
			if err != nil {
				b.Fatal(err)
			}
			path := "/api/v1/workspaces/" + ws.ID.String() + "/nodes/" + page.ID.String() + "/assets"
			b.ReportAllocs()
			b.SetBytes(size)
			b.ResetTimer()
			for range b.N {
				f, err := os.Open(bodyPath) //nolint:gosec // G304: Benchmark-owned temporary path.
				if err != nil {
					b.Fatal(err)
				}
				code := uploadQuotaAsset(b, e, token, path, w.FormDataContentType(), f, info.Size())
				if code != sample.status {
					b.Fatalf("upload: %d", code)
				}
			}
		})
	}
}
