// Tests parent-session asset links across credential transports, revocation, expiry, and membership loss.

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/reqctx"
	"github.com/maruel/mddb/backend/internal/storage"
)

func TestSessionAssets(t *testing.T) {
	t.Parallel()
	env := setupTestEnv(t)
	var login dto.AuthResponse
	if status := env.doJSON(t, "POST", "/api/v1/auth/register", dto.RegisterRequest{Email: "assets@example.com", Password: "Pass1234", Name: "Owner"}, &login, ""); status != 200 {
		t.Fatal(status)
	}
	var org dto.OrganizationResponse
	if status := env.doJSON(t, "POST", "/api/v1/organizations", dto.CreateOrganizationRequest{Name: "Assets"}, &org, login.Token); status != 200 {
		t.Fatal(status)
	}
	var ws dto.WorkspaceResponse
	if status := env.doJSON(t, "POST", "/api/v1/organizations/"+org.ID.String()+"/workspaces", dto.CreateWorkspaceRequest{Name: "Assets"}, &ws, login.Token); status != 200 {
		t.Fatal(status)
	}
	user, err := env.services.User.Get(login.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	var page dto.CreatePageResponse
	prefix := "/api/v1/workspaces/" + ws.ID.String() + "/nodes/"
	if status := env.doJSON(t, "POST", prefix+"0/page/create", dto.CreatePageRequest{Title: "Photo", Content: "![Photo](photo.png)"}, &page, login.Token); status != 200 {
		t.Fatal(status)
	}
	store, err := env.services.FileStore.GetWorkspaceStore(t.Context(), ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveAsset(t.Context(), page.ID, "photo #1.png", []byte("image data"), handlers.GitAuthor(user)); err != nil {
		t.Fatal(err)
	}
	cfg := &handlers.Config{ServerConfig: storage.ServerConfig{JWTSecret: testJWTSecret}}
	request := func(method, path, body, token string, cookie bool) (int, []byte, http.Header) {
		r, err := http.NewRequestWithContext(t.Context(), method, env.server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", env.server.URL)
		if cookie {
			r.Header.Set("Cookie", handlers.SessionCookieName+"="+token)
		} else if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := env.server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := response.Body.Close(); err != nil {
				t.Error(err)
			}
		}()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, data, response.Header
	}
	token, sid, err := cfg.IssueToken(env.services.Session, user, "", "Browser", "")
	if err != nil {
		t.Fatal(err)
	}
	var link string
	for _, cookie := range []bool{false, true} {
		status, data, _ := request("GET", prefix+page.ID.String(), "", token, cookie)
		if status != 200 {
			t.Fatal(status, string(data))
		}
		var node dto.NodeResponse
		if err := json.Unmarshal(data, &node); err != nil {
			t.Fatal(err)
		}
		link = node.AssetURLs["photo #1.png"]
		if link == "" {
			t.Fatal("missing parent-bound image link")
		}
		status, data, headers := request("GET", link, "", "", false)
		if status != 200 || string(data) != "image data" || headers.Get("Cache-Control") != "no-store" {
			t.Fatalf("image=%d %s", status, data)
		}
		if status, _, _ := request("GET", strings.Replace(link, "sid="+sid.String(), "sid="+login.User.SessionID.String(), 1), "", "", false); status != 403 {
			t.Fatal("tampered parent accepted", status)
		}
	}
	if err := env.services.Session.Revoke(sid); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := request("GET", link, "", "", false); status != 403 {
		t.Fatal("asset survived revocation", status)
	}
	for _, cookie := range []bool{false, true} {
		if status, _, _ := request("GET", "/api/v1/auth/me", "", token, cookie); status != 401 {
			t.Fatal("credential survived revoke", status)
		}
	}
	expired, err := env.services.Session.Create(user.ID, "expired-hash", "Browser", "", "", storage.ToTime(time.Now().Add(-time.Hour)), 0)
	if err != nil {
		t.Fatal(err)
	}
	link = cfg.GenerateSignedAssetURL(reqctx.WithSession(t.Context(), expired), ws.ID, page.ID, "photo #1.png")
	if status, _, _ := request("GET", link, "", "", false); status != 403 {
		t.Fatal("asset survived expiry", status)
	}
	// Existing ordinary owner reads still issue working images; losing membership revokes the link.
	status, data, _ := request("GET", prefix+page.ID.String(), "", login.Token, false)
	if status != 200 {
		t.Fatal(status)
	}
	var node dto.NodeResponse
	if err := json.Unmarshal(data, &node); err != nil {
		t.Fatal(err)
	}
	membership, err := env.services.OrgMembership.Get(user.ID, org.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.services.OrgMembership.Delete(membership.ID); err != nil {
		t.Fatal(err)
	}
	if status, _, _ := request("GET", node.AssetURLs["photo #1.png"], "", "", false); status != 403 {
		t.Fatal("asset survived membership removal", status)
	}
}
