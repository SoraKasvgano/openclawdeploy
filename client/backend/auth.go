package backend

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"openclawdeploy/internal/shared"
)

const clientSessionCookieName = "openclawdeploy_client_session"
const clientSessionTTL = 24 * time.Hour

func (a *App) requireLocalAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.isLocalAuthorized(r) {
			shared.WriteError(w, http.StatusUnauthorized, "client authentication required")
			return
		}
		next(w, r)
	}
}

func (a *App) isLocalAuthorized(r *http.Request) bool {
	cookie, err := r.Cookie(clientSessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return false
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.sessionToken == "" || time.Now().After(a.sessionExpiry) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(a.sessionToken)) == 1
}

func (a *App) verifyLocalCredentials(username, password string) bool {
	cfg := a.configSnapshot()
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	return subtle.ConstantTimeCompare([]byte(username), []byte(strings.TrimSpace(cfg.WebUsername))) == 1 &&
		subtle.ConstantTimeCompare([]byte(password), []byte(strings.TrimSpace(cfg.WebPassword))) == 1
}

func (a *App) issueLocalSession(w http.ResponseWriter) error {
	token, err := randomLocalToken()
	if err != nil {
		return err
	}

	expiresAt := time.Now().Add(clientSessionTTL)

	a.mu.Lock()
	a.sessionToken = token
	a.sessionExpiry = expiresAt
	a.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     clientSessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (a *App) clearLocalSession(w http.ResponseWriter) {
	a.mu.Lock()
	a.sessionToken = ""
	a.sessionExpiry = time.Time{}
	a.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     clientSessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func randomLocalToken() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
