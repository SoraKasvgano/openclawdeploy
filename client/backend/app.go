package backend

import (
	"context"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type App struct {
	cfg      *Config
	logger   *log.Logger
	openclaw *OpenClawManager
	syncer   *Syncer
	server   *http.Server
	mu       sync.RWMutex

	sessionToken  string
	sessionExpiry time.Time
}

func New() (*App, error) {
	logger := log.New(os.Stdout, "[client] ", log.LstdFlags)

	cfg, err := LoadOrCreateConfig()
	if err != nil {
		return nil, err
	}

	openclaw := NewOpenClawManager(cfg.OpenClawConfigPath, logger)
	if err := openclaw.EnsureFile(); err != nil {
		return nil, err
	}

	if err := EnsureDeployScript(logger); err != nil {
		return nil, err
	}

	logger.Printf("device id: %s", cfg.DeviceID)
	logger.Printf("device qr code:\n%s", RenderIdentityQRCodeCLI(cfg.DeviceID))

	app := &App{
		cfg:      cfg,
		logger:   logger,
		openclaw: openclaw,
	}
	app.syncer = NewSyncer(app.configSnapshot, openclaw)
	app.server = &http.Server{
		Addr:              cfg.Address(),
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	go a.syncer.Start(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = a.server.Shutdown(shutdownCtx)
	}()

	a.logger.Printf("client web ui listening on http://127.0.0.1:%d", a.cfg.WebPort)
	return a.server.ListenAndServe()
}

func (a *App) configSnapshot() Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return *a.cfg
}

func (a *App) updateConfig(update func(*Config)) (Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	update(a.cfg)
	if err := a.cfg.Save(); err != nil {
		return Config{}, err
	}
	return *a.cfg, nil
}
