package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/lambdawp-567/sharedgpupower/backend/internal/users"
	"go.uber.org/zap"
)

type contextKey string

const (
	userContextKey contextKey = "user"
)

// Introspector validates an OIDC token and returns the subject (Zitadel user ID),
// email, name, and avatar URL.
type Introspector interface {
	Introspect(ctx context.Context, token string) (zitadelID, email, name, avatarURL string, err error)
}

// Middleware validates Zitadel JWT tokens and loads the local user record.
type Middleware struct {
	introspector Introspector
	users        *users.Store
	log          *zap.Logger
}

func NewMiddleware(introspector Introspector, users *users.Store, log *zap.Logger) *Middleware {
	return &Middleware{introspector: introspector, users: users, log: log}
}

// Authenticate validates the Bearer token and injects the user into the context.
// Returns 401 if the token is missing/invalid, 403 if the user is pending/disabled.
func (m *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			// Also check cookie
			if c, err := r.Cookie("sgpu_session"); err == nil {
				token = c.Value
			}
		}
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		zitadelID, email, name, avatarURL, err := m.introspector.Introspect(r.Context(), token)
		if err != nil {
			m.log.Debug("token introspection failed", zap.Error(err))
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		user, _, err := m.users.FindOrCreate(r.Context(), zitadelID, email, name, avatarURL)
		if err != nil {
			m.log.Error("find or create user failed", zap.Error(err))
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireActive rejects users with status != 'active'.
func RequireActive(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFromContext(r.Context())
		if u == nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if u.Status != "active" {
			http.Error(w, "account pending approval", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin rejects non-admin users.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFromContext(r.Context())
		if u == nil || u.Role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UserFromContext retrieves the authenticated user from the request context.
func UserFromContext(ctx context.Context) *users.User {
	u, _ := ctx.Value(userContextKey).(*users.User)
	return u
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}
