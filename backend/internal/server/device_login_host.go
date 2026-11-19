// Adapts device pairing to mddb accounts, session delivery and revocation, and durable audits.

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	identityv1 "github.com/maruel/mddb/backend/internal/storage/identity/data/v1"

	serverv1 "github.com/maruel/mddb/backend/internal/server/data/v1"

	"github.com/gomodedev/gomode/devicelogin"
	"github.com/gomodedev/gomode/httpserver"
	"github.com/maruel/ksid"
	"github.com/maruel/mddb/backend/internal/server/dto/v1"
	"github.com/maruel/mddb/backend/internal/server/handlers"
)

type deviceLoginHost struct {
	services *handlers.Services
	config   *Config
	auth     *handlers.AuthHandler

	// mu serializes the process-owned audit file.
	mu sync.Mutex
}

func (h *deviceLoginHost) Enabled() bool {
	return len(h.config.JWTSecret) >= 32 && h.services.User != nil && h.services.Session != nil && h.config.ConfigDir != ""
}

func (h *deviceLoginHost) ClientIP(r *http.Request) netip.Addr {
	ip, _ := netip.ParseAddr(httpserver.ClientIP(r.Context()))
	return ip
}

func (h *deviceLoginHost) VerificationURL(_ *http.Request) string {
	return strings.TrimRight(h.config.BaseURL, "/") + "/device"
}

func (h *deviceLoginHost) Identify(r *http.Request) (string, error) {
	u, sid, _, err := validateJWTAndSession(r, h.services.User, h.services.Session, h.config.JWTSecret)
	if err != nil {
		return "", err
	}
	// Bind the shared CSRF proof to the exact authenticated session.
	return u.ID.String() + "\x00" + sid.String(), nil
}

func (h *deviceLoginHost) Eligible(_ context.Context, subject string) error {
	id, err := deviceApproverUserID(subject)
	if err != nil {
		return err
	}
	_, err = h.services.User.Get(id)
	return err
}

func (h *deviceLoginHost) CreateSession(r *http.Request, subject string) (devicelogin.Session, error) {
	id, err := deviceApproverUserID(subject)
	if err != nil {
		return nil, err
	}
	user, err := h.services.User.Get(id)
	if err != nil {
		return nil, err
	}
	defer commitDBIfMutating(r.Context(), r, h.services.RootRepo, handlers.GitAuthor(user))
	response, sid, err := h.auth.IssueSession(r.Context(), user)
	if sid == 0 {
		return nil, err
	}
	return &deviceSession{response: response, id: sid, services: h.services, request: r, user: user, config: &handlers.Config{ServerConfig: *h.config.ServerConfig, BaseURL: h.config.BaseURL}}, err
}

func (h *deviceLoginHost) Audit(_ context.Context, event *devicelogin.AuditEvent) error {
	id, err := deviceApproverUserID(event.Subject)
	if err != nil {
		return err
	}
	// Go Mode audits decisions and creation before exposing credentials.
	h.mu.Lock()
	defer h.mu.Unlock()
	data, err := json.Marshal(serverv1.DeviceLoginAudit{
		RequestID: event.RequestID, SessionID: event.SessionID, Kind: event.Kind,
		Subject: id.String(), Approved: event.Approved, IP: event.IP,
		UserAgent: event.UserAgent, RequestedAt: event.CreatedAt, DecidedAt: time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(h.config.ConfigDir, "device-login-audit.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		return errors.Join(err, f.Close())
	}
	_, err = f.Write(append(data, '\n'))
	return errors.Join(err, f.Sync(), f.Close())
}

type deviceSession struct {
	response *dto.AuthResponse
	id       ksid.ID
	services *handlers.Services
	request  *http.Request
	user     *identityv1.User
	config   *handlers.Config
}

func (s *deviceSession) SessionID() string { return s.id.String() }
func (s *deviceSession) WriteResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	if s.request.URL.Path == browserDevicePoll {
		if err := s.config.WriteSessionCookie(w, s.response.Token); err != nil {
			return err
		}
		return json.NewEncoder(w).Encode(s.response.User)
	}
	return json.NewEncoder(w).Encode(s.response)
}
func (s *deviceSession) Revoke(ctx context.Context) error {
	defer commitDBIfMutating(ctx, s.request, s.services.RootRepo, handlers.GitAuthor(s.user))
	return s.services.Session.Revoke(s.id)
}

// deviceApproverUserID decodes the host account and audit identity from the session-bound subject.
func deviceApproverUserID(subject string) (ksid.ID, error) {
	id, sid, ok := strings.Cut(subject, "\x00")
	if !ok || sid == "" {
		return 0, errors.New("invalid approving session")
	}
	return ksid.Parse(id)
}
