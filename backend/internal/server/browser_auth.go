// Establishes browser-only sessions without exposing credentials to JavaScript or redirect URLs.

package server

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/maruel/mddb/backend/internal/server/dto"
	"github.com/maruel/mddb/backend/internal/server/handlers"
	"github.com/maruel/mddb/backend/internal/server/ratelimit"
	"github.com/maruel/mddb/backend/internal/storage/identity"
)

const browserDevicePoll = "/api/v1/auth/browser/device/poll"

func wrapBrowserLogin[In any, PtrIn interface {
	*In
	dto.Validatable
}](fn func(context.Context, PtrIn) (*dto.AuthResponse, error), svc *handlers.Services, cfg *handlers.Config, limiters *ratelimit.Limiters) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WrapWithSvc(func(ctx context.Context, req PtrIn) (*dto.UserResponse, error) {
			response, err := fn(ctx, req)
			if err != nil {
				return nil, err
			}
			if err := cfg.WriteSessionCookie(w, response.Token); err != nil {
				return nil, err
			}
			return response.User, nil
		}, svc, cfg, limiters).ServeHTTP(w, r)
	})
}

func wrapBrowserLogout(authh *handlers.AuthHandler, svc *handlers.Services, cfg *handlers.Config, limiters *ratelimit.Limiters) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WrapAuth(func(ctx context.Context, u *identity.User, req *dto.LogoutRequest) (*dto.LogoutResponse, error) {
			out, err := authh.Logout(ctx, u, req)
			if err == nil {
				handlers.ClearSessionCookie(w)
			}
			return out, err
		}, svc, cfg, limiters).ServeHTTP(w, r)
	})
}

// requestCredential chooses one authoritative session; a second credential may
// repeat it but must never select a different session, even for the same user.
func requestCredential(r *http.Request) (string, error) {
	var cookie string
	count := 0
	for _, c := range r.Cookies() {
		if c.Name == handlers.SessionCookieName {
			cookie = c.Value
			count++
		}
	}
	if count > 1 || (count == 1 && cookie == "") {
		return "", errInvalidAuthHdr
	}
	auth := r.Header.Values("Authorization")
	if len(auth) > 1 {
		return "", errInvalidAuthHdr
	}
	bearer := ""
	if len(auth) == 1 {
		value := auth[0]
		if len(value) <= 7 || value[:7] != "Bearer " {
			return "", errInvalidAuthHdr
		}
		bearer = value[7:]
	}
	if cookie != "" && bearer != "" && subtle.ConstantTimeCompare([]byte(cookie), []byte(bearer)) != 1 {
		return "", errInvalidAuthHdr
	}
	if cookie != "" {
		return cookie, nil
	}
	if bearer != "" {
		return bearer, nil
	}
	return "", errUnauthorized
}
