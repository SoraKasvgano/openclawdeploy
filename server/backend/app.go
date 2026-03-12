package backend

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

const configWatchInterval = time.Second

type App struct {
	cfg    Config
	store  *Store
	logger *log.Logger
	mu     sync.RWMutex
}

type activeServer struct {
	cfg    Config
	server *http.Server
	errCh  chan error
}

type configStamp struct {
	found   bool
	modTime time.Time
	size    int64
}

func New() (*App, error) {
	logger := log.New(os.Stdout, "[server] ", log.LstdFlags)

	cfg, err := LoadOrCreateConfig()
	if err != nil {
		return nil, err
	}

	store, err := NewStore(cfg.StatePath, logger)
	if err != nil {
		return nil, err
	}

	return &App{
		cfg:    *cfg,
		store:  store,
		logger: logger,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	defer func() {
		if err := a.store.Close(); err != nil {
			a.logger.Printf("close state store failed: %v", err)
		}
	}()

	cfg := a.configSnapshot()
	active, err := a.startServer(cfg)
	if err != nil {
		return err
	}

	a.logger.Printf("server ui listening on http://127.0.0.1:%d", cfg.WebPort)
	a.logger.Printf("server config file: %s", cfg.ConfigPath)
	a.logger.Printf("default admin credential: admin / admin")

	reloadCh := a.watchConfig(ctx, cfg.ConfigPath)
	for {
		select {
		case <-ctx.Done():
			return a.shutdownServer(active)
		case err := <-active.errCh:
			if err != nil {
				return err
			}
			return nil
		case nextCfg, ok := <-reloadCh:
			if !ok {
				reloadCh = nil
				continue
			}
			active = a.reloadServer(active, nextCfg)
		}
	}
}

func (a *App) configSnapshot() Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

func (a *App) setConfig(cfg Config) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.cfg = cfg
}

func (a *App) updateConfig(update func(*Config)) (Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	next := a.cfg
	update(&next)
	if err := next.Save(); err != nil {
		return Config{}, err
	}
	a.cfg = next
	return next, nil
}

func (a *App) startServer(cfg Config) (*activeServer, error) {
	listener, err := net.Listen("tcp", cfg.Address())
	if err != nil {
		return nil, err
	}
	return a.serveListener(cfg, listener), nil
}

func (a *App) serveListener(cfg Config, listener net.Listener) *activeServer {
	server := &http.Server{
		Addr:              cfg.Address(),
		Handler:           a.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	active := &activeServer{
		cfg:    cfg,
		server: server,
		errCh:  make(chan error, 1),
	}

	go func() {
		err := server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			active.errCh <- err
			return
		}
		active.errCh <- nil
	}()

	return active
}

func (a *App) shutdownServer(active *activeServer) error {
	if active == nil || active.server == nil {
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := active.server.Shutdown(shutdownCtx)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (a *App) reloadServer(active *activeServer, nextCfg Config) *activeServer {
	currentCfg := a.configSnapshot()
	if currentCfg.Equals(nextCfg) {
		return active
	}

	if active != nil && active.cfg.Address() != nextCfg.Address() {
		listener, err := net.Listen("tcp", nextCfg.Address())
		if err != nil {
			a.logger.Printf("server config reload skipped: listen on %s failed: %v", nextCfg.Address(), err)
			return active
		}

		a.setConfig(nextCfg)
		replacement := a.serveListener(nextCfg, listener)
		a.logger.Printf("server config reloaded: now listening on http://127.0.0.1:%d", nextCfg.WebPort)
		go func(previous *activeServer) {
			_ = a.shutdownServer(previous)
		}(active)
		return replacement
	}

	a.setConfig(nextCfg)
	if active != nil {
		active.cfg = nextCfg
	}
	a.logger.Printf("server config reloaded without listener restart")
	return active
}

func (a *App) watchConfig(ctx context.Context, path string) <-chan Config {
	updates := make(chan Config)

	go func() {
		defer close(updates)

		ticker := time.NewTicker(configWatchInterval)
		defer ticker.Stop()

		lastStamp := currentConfigStamp(path)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stamp := currentConfigStamp(path)
				if stamp == lastStamp {
					continue
				}
				lastStamp = stamp

				cfg, err := LoadConfigFromPath(path)
				if err != nil {
					a.logger.Printf("server config reload skipped: %v", err)
					continue
				}

				select {
				case updates <- *cfg:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return updates
}

func currentConfigStamp(path string) configStamp {
	info, err := os.Stat(path)
	if err != nil {
		return configStamp{}
	}
	return configStamp{
		found:   true,
		modTime: info.ModTime(),
		size:    info.Size(),
	}
}
