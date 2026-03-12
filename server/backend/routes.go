package backend

import (
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"openclawdeploy/internal/shared"
	"openclawdeploy/server/frontend"
)

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /openapi.json", a.handleOpenAPI)
	mux.HandleFunc("GET /swagger", a.handleSwaggerRedirect)
	mux.HandleFunc("GET /swagger/{$}", a.handleSwaggerIndex)
	mux.HandleFunc("GET /swagger/index.html", a.handleSwaggerIndex)
	mux.HandleFunc("GET /swagger/{asset}", a.handleSwaggerAsset)

	mux.HandleFunc("GET /api/v1/settings/public", a.handlePublicSettings)
	mux.HandleFunc("POST /api/v1/auth/register", a.handleRegister)
	mux.HandleFunc("POST /api/v1/auth/login", a.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/forgot-password", a.handleForgotPassword)
	mux.HandleFunc("POST /api/v1/auth/reset-password", a.handleResetPassword)
	mux.HandleFunc("POST /api/v1/client/heartbeat", a.handleClientHeartbeat)

	mux.HandleFunc("GET /api/v1/auth/me", a.requireUser(false, a.handleMe))
	mux.HandleFunc("PUT /api/v1/auth/profile", a.requireUser(false, a.handleUpdateProfile))
	mux.HandleFunc("POST /api/v1/auth/logout", a.requireUser(false, a.handleLogout))
	mux.HandleFunc("GET /api/v1/devices", a.requireUser(false, a.handleListDevices))
	mux.HandleFunc("POST /api/v1/devices/bind", a.requireUser(false, a.handleBindDevice))
	mux.HandleFunc("DELETE /api/v1/devices/{deviceID}", a.requireUser(false, a.handleDeleteDevice))
	mux.HandleFunc("PUT /api/v1/devices/{deviceID}/remark", a.requireUser(false, a.handleUpdateDeviceRemark))
	mux.HandleFunc("PUT /api/v1/devices/{deviceID}/config", a.requireUser(false, a.handleUpdateDeviceConfig))

	mux.HandleFunc("GET /api/v1/admin/summary", a.requireUser(true, a.handleSummary))
	mux.HandleFunc("GET /api/v1/admin/settings", a.requireUser(true, a.handleAdminSettings))
	mux.HandleFunc("GET /api/v1/admin/users", a.requireUser(true, a.handleListUsers))
	mux.HandleFunc("POST /api/v1/admin/users", a.requireUser(true, a.handleCreateUser))
	mux.HandleFunc("PUT /api/v1/admin/users/{id}", a.requireUser(true, a.handleUpdateUser))
	mux.HandleFunc("DELETE /api/v1/admin/users/{id}", a.requireUser(true, a.handleDeleteUser))
	mux.HandleFunc("POST /api/v1/admin/settings/registration", a.requireUser(true, a.handleSetRegistration))
	mux.HandleFunc("POST /api/v1/admin/settings/smtp", a.requireUser(true, a.handleSetSMTP))

	mux.Handle("/", a.frontendHandler())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirected := normalizeDocsPath(r.URL.Path); redirected != "" && redirected != r.URL.Path {
			http.Redirect(w, r, redirected, http.StatusFound)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (a *App) frontendHandler() http.Handler {
	fileServer := http.FileServer(http.FS(frontend.StaticFS()))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/index.html", "/app.js", "/style.css":
			fileServer.ServeHTTP(w, r)
		default:
			if strings.HasPrefix(r.URL.Path, "/api/") {
				shared.WriteError(w, http.StatusNotFound, "route not found")
				return
			}
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		}
	})
}

func (a *App) handlePublicSettings(w http.ResponseWriter, r *http.Request) {
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"registration_enabled": a.store.RegistrationEnabled(),
		"swagger_url":          "/swagger/",
		"api_token_header":     apiTokenHeaderName,
	})
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !a.store.RegistrationEnabled() {
		shared.WriteError(w, http.StatusForbidden, "registration is disabled")
		return
	}

	var payload struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := a.store.CreateUser(payload.Username, payload.Email, payload.Password, false)
	if err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	shared.WriteJSON(w, http.StatusCreated, map[string]any{
		"user":    user,
		"message": "registered",
	})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := a.store.Authenticate(payload.Username, payload.Password)
	if err != nil {
		shared.WriteError(w, http.StatusUnauthorized, err.Error())
		return
	}

	sessionTTL := time.Duration(a.configSnapshot().SessionTTLHours) * time.Hour
	session, err := a.store.CreateSession(user.ID, sessionTTL)
	if err != nil {
		shared.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.setSessionCookie(w, session)
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"token": session.Token,
		"user":  a.store.publicUserLocked(user),
	})
}

func (a *App) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Identifier string `json:"identifier"`
	}
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, token, err := a.store.IssueResetToken(payload.Identifier)
	if err == nil {
		link := a.publicBaseURL(r) + "/?reset_token=" + url.QueryEscape(token)
		if mailErr := a.sendResetEmail(user.Email, link); mailErr != nil {
			a.logger.Printf("send reset email failed: %v", mailErr)
		}
	}

	shared.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "if the account exists, a reset link has been sent",
	})
}

func (a *App) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.store.ResetPassword(strings.TrimSpace(payload.Token), payload.NewPassword); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "password updated",
	})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request, user User) {
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"user": a.store.publicUserLocked(user),
	})
}

func (a *App) handleUpdateProfile(w http.ResponseWriter, r *http.Request, user User) {
	if user.ID == a.apiActor().ID {
		shared.WriteError(w, http.StatusForbidden, "api token actor cannot update profile")
		return
	}

	var payload struct {
		Email    *string `json:"email"`
		Password *string `json:"password"`
	}
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := a.store.UpdateUser(user.ID, UpdateUserInput{
		Email:    payload.Email,
		Password: payload.Password,
	})
	if err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"user":    updated,
		"message": "profile updated",
	})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request, user User) {
	if a.isAPITokenAuthorized(r) {
		shared.WriteJSON(w, http.StatusOK, map[string]string{
			"message": "api token calls do not require logout",
		})
		return
	}

	token := a.tokenFromRequest(r)
	if token != "" {
		_ = a.store.DestroySession(token)
	}
	a.clearSessionCookie(w)
	shared.WriteJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

func (a *App) handleClientHeartbeat(w http.ResponseWriter, r *http.Request) {
	var payload ClientHeartbeatRequest
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	response, err := a.store.HandleHeartbeat(payload, remoteIP(r))
	if err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	shared.WriteJSON(w, http.StatusOK, response)
}

func (a *App) handleListDevices(w http.ResponseWriter, r *http.Request, user User) {
	filter := ""
	if user.IsAdmin {
		filter = strings.TrimSpace(r.URL.Query().Get("owner_username"))
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"devices": a.store.ListDevicesByOwnerUsername(user, filter),
	})
}

func (a *App) handleBindDevice(w http.ResponseWriter, r *http.Request, user User) {
	var payload struct {
		DeviceID string `json:"device_id"`
	}
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	device, err := a.store.BindDevice(strings.TrimSpace(payload.DeviceID), user.ID)
	if err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"device":  device,
		"message": "device bound",
	})
}

func (a *App) handleUpdateDeviceRemark(w http.ResponseWriter, r *http.Request, user User) {
	var payload struct {
		Remark string `json:"remark"`
	}
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	device, err := a.store.UpdateDeviceRemark(r.PathValue("deviceID"), user, payload.Remark)
	if err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"device":  device,
		"message": "remark updated",
	})
}

func (a *App) handleUpdateDeviceConfig(w http.ResponseWriter, r *http.Request, user User) {
	var payload struct {
		OpenClawJSON string `json:"openclaw_json"`
	}
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	device, err := a.store.UpdateDeviceConfig(r.PathValue("deviceID"), user, payload.OpenClawJSON)
	if err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"device":  device,
		"message": "config scheduled",
	})
}

func (a *App) handleDeleteDevice(w http.ResponseWriter, r *http.Request, user User) {
	if err := a.store.DeleteDevice(r.PathValue("deviceID"), user); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "device deleted",
	})
}

func (a *App) handleSummary(w http.ResponseWriter, r *http.Request, user User) {
	shared.WriteJSON(w, http.StatusOK, a.store.Summary())
}

func (a *App) handleAdminSettings(w http.ResponseWriter, r *http.Request, user User) {
	cfg := a.configSnapshot()

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"registration_enabled": a.store.RegistrationEnabled(),
		"smtp":                 cfg.SMTP,
		"api_token_header":     apiTokenHeaderName,
		"swagger_url":          "/swagger/",
		"listen_addr":          cfg.ListenAddr,
		"web_port":             cfg.WebPort,
		"config_file":          filepath.Base(cfg.ConfigPath),
		"hot_reload":           true,
	})
}

func (a *App) handleListUsers(w http.ResponseWriter, r *http.Request, user User) {
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"users": a.store.ListUsers(),
	})
}

func (a *App) handleCreateUser(w http.ResponseWriter, r *http.Request, user User) {
	var payload struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := a.store.CreateUser(payload.Username, payload.Email, payload.Password, payload.IsAdmin)
	if err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	shared.WriteJSON(w, http.StatusCreated, map[string]any{
		"user":    created,
		"message": "user created",
	})
}

func (a *App) handleUpdateUser(w http.ResponseWriter, r *http.Request, user User) {
	var payload UpdateUserInput
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := a.store.UpdateUser(r.PathValue("id"), payload)
	if err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"user":    updated,
		"message": "user updated",
	})
}

func (a *App) handleDeleteUser(w http.ResponseWriter, r *http.Request, user User) {
	if err := a.store.DeleteUser(r.PathValue("id")); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "user deleted",
	})
}

func (a *App) handleSetRegistration(w http.ResponseWriter, r *http.Request, user User) {
	var payload struct {
		Enabled bool `json:"enabled"`
	}
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.store.SetRegistrationEnabled(payload.Enabled); err != nil {
		shared.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]bool{
		"registration_enabled": payload.Enabled,
	})
}

func (a *App) handleSetSMTP(w http.ResponseWriter, r *http.Request, user User) {
	var payload SMTPConfig
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	payload.Host = strings.TrimSpace(payload.Host)
	payload.Username = strings.TrimSpace(payload.Username)
	payload.Password = strings.TrimSpace(payload.Password)
	payload.From = strings.TrimSpace(payload.From)
	if payload.Port <= 0 {
		payload.Port = 25
	}

	cfg, err := a.updateConfig(func(next *Config) {
		next.SMTP = payload
	})
	if err != nil {
		shared.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"message": "smtp settings updated",
		"smtp":    cfg.SMTP,
	})
}

func (a *App) publicBaseURL(r *http.Request) string {
	cfg := a.configSnapshot()
	if strings.TrimSpace(cfg.PublicBaseURL) != "" {
		return strings.TrimRight(cfg.PublicBaseURL, "/")
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func remoteIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func normalizeDocsPath(path string) string {
	switch {
	case strings.EqualFold(path, "/swagger"):
		return "/swagger/"
	case strings.EqualFold(path, "/swagger/"):
		return "/swagger/"
	case strings.EqualFold(path, "/swagger/index.html"):
		return "/swagger/index.html"
	case len(path) > len("/swagger/") && strings.EqualFold(path[:len("/swagger/")], "/swagger/"):
		return "/swagger/" + path[len("/swagger/"):]
	case strings.EqualFold(path, "/openapi.json"):
		return "/openapi.json"
	default:
		return ""
	}
}
