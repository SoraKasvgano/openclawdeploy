package backend

import (
	"context"
	"net/http"
	"strings"
	"time"

	"openclawdeploy/client/frontend"
	"openclawdeploy/internal/shared"
)

type statusResponse struct {
	DeviceID          string       `json:"device_id"`
	Identity          Identity     `json:"identity"`
	IdentityMatrixSVG string       `json:"identity_matrix_svg"`
	Config            configView   `json:"config"`
	ConfigPath        string       `json:"config_path"`
	ListenAddress     string       `json:"listen_address"`
	OpenClawPath      string       `json:"openclaw_path"`
	OpenClawJSON      string       `json:"openclaw_json"`
	Sync              SyncSnapshot `json:"sync"`
}

type configView struct {
	WebPort             int    `json:"web_port"`
	ListenAddr          string `json:"listen_addr"`
	ServerURL           string `json:"server_url"`
	SyncIntervalSeconds int    `json:"sync_interval_seconds"`
	OpenClawConfigPath  string `json:"openclaw_config_path"`
	AllowRemoteReboot   bool   `json:"allow_remote_reboot"`
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/client/auth/login", a.handleClientLogin)
	mux.HandleFunc("GET /api/v1/client/auth/me", a.requireLocalAuth(a.handleClientMe))
	mux.HandleFunc("POST /api/v1/client/auth/logout", a.requireLocalAuth(a.handleClientLogout))
	mux.HandleFunc("POST /api/v1/client/auth/account", a.requireLocalAuth(a.handleClientAccount))
	mux.HandleFunc("GET /api/v1/client/status", a.requireLocalAuth(a.handleStatus))
	mux.HandleFunc("POST /api/v1/client/openclaw", a.requireLocalAuth(a.handleWriteOpenClaw))
	mux.HandleFunc("POST /api/v1/client/sync", a.requireLocalAuth(a.handleSyncNow))
	mux.Handle("/", a.frontendHandler())
	return mux
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

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	cfg := a.configSnapshot()
	openclawJSON, err := a.openclaw.Read()
	if err != nil {
		shared.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	networkOK := checkNetwork(r.Context())
	identity := CurrentIdentity(&cfg, networkOK)
	shared.WriteJSON(w, http.StatusOK, statusResponse{
		DeviceID:          cfg.DeviceID,
		Identity:          identity,
		IdentityMatrixSVG: RenderIdentityMatrixSVG(cfg.DeviceID),
		Config:            configViewFromConfig(cfg),
		ConfigPath:        cfg.ConfigPath,
		ListenAddress:     cfg.Address(),
		OpenClawPath:      a.openclaw.Path(),
		OpenClawJSON:      openclawJSON,
		Sync:              a.syncer.Snapshot(),
	})
}

func (a *App) handleWriteOpenClaw(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		OpenClawJSON string `json:"openclaw_json"`
	}
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	changed, err := a.openclaw.Apply(payload.OpenClawJSON)
	if err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if changed {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := a.openclaw.RestartGateway(ctx); err != nil {
			shared.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"changed": changed,
		"message": "本地配置已保存",
	})
}

func (a *App) handleSyncNow(w http.ResponseWriter, r *http.Request) {
	if err := a.syncer.SyncNow(r.Context()); err != nil {
		shared.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]string{
		"message": a.syncer.Snapshot().LastSyncMessage,
	})
}

func (a *App) handleClientLogin(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !a.verifyLocalCredentials(payload.Username, payload.Password) {
		shared.WriteError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}
	if err := a.issueLocalSession(w); err != nil {
		shared.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	cfg := a.configSnapshot()
	shared.WriteJSON(w, http.StatusOK, map[string]string{
		"username": cfg.WebUsername,
		"message":  "login success",
	})
}

func (a *App) handleClientMe(w http.ResponseWriter, r *http.Request) {
	cfg := a.configSnapshot()
	shared.WriteJSON(w, http.StatusOK, map[string]string{
		"username": cfg.WebUsername,
	})
}

func (a *App) handleClientLogout(w http.ResponseWriter, r *http.Request) {
	a.clearLocalSession(w)
	shared.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "logout success",
	})
}

func (a *App) handleClientAccount(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := shared.ReadJSON(r, &payload); err != nil {
		shared.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	username := strings.TrimSpace(payload.Username)
	password := strings.TrimSpace(payload.Password)
	if username == "" {
		shared.WriteError(w, http.StatusBadRequest, "username is required")
		return
	}

	cfg, err := a.updateConfig(func(next *Config) {
		next.WebUsername = username
		if password != "" {
			next.WebPassword = password
		}
	})
	if err != nil {
		shared.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]string{
		"username": cfg.WebUsername,
		"message":  "account updated",
	})
}

func configViewFromConfig(cfg Config) configView {
	return configView{
		WebPort:             cfg.WebPort,
		ListenAddr:          cfg.ListenAddr,
		ServerURL:           cfg.ServerURL,
		SyncIntervalSeconds: cfg.SyncIntervalSeconds,
		OpenClawConfigPath:  cfg.OpenClawConfigPath,
		AllowRemoteReboot:   cfg.AllowRemoteReboot,
	}
}
