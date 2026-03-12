package backend

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"time"

	"openclawdeploy/internal/shared"
)

const sessionCookieName = "openclawdeploy_session"
const apiTokenHeaderName = "X-API-Token"

func (a *App) tokenFromRequest(r *http.Request) string {
	if token := strings.TrimSpace(r.Header.Get(apiTokenHeaderName)); token != "" {
		return token
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		return cookie.Value
	}
	return ""
}

func (a *App) isAPITokenAuthorized(r *http.Request) bool {
	token := a.tokenFromRequest(r)
	cfg := a.configSnapshot()
	if token == "" || cfg.AIToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(cfg.AIToken)) == 1
}

func (a *App) apiActor() User {
	return User{
		ID:        "ai-token",
		Username:  "ai-token",
		Email:     "",
		IsAdmin:   true,
		CreatedAt: "",
		UpdatedAt: "",
	}
}

func (a *App) requireUser(adminOnly bool, next func(http.ResponseWriter, *http.Request, User)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := a.tokenFromRequest(r)
		if token == "" {
			shared.WriteError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if a.isAPITokenAuthorized(r) {
			next(w, r, a.apiActor())
			return
		}

		user, ok := a.store.GetUserBySession(token)
		if !ok {
			shared.WriteError(w, http.StatusUnauthorized, "session expired")
			return
		}
		if adminOnly && !user.IsAdmin {
			shared.WriteError(w, http.StatusForbidden, "admin permission required")
			return
		}
		next(w, r, user)
	}
}

func (a *App) setSessionCookie(w http.ResponseWriter, session Session) {
	expiresAt, err := time.Parse(time.RFC3339, session.ExpiresAt)
	if err != nil {
		expiresAt = time.Now().Add(72 * time.Hour)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
}

func (a *App) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
