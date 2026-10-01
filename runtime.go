package main

import (
	"context"
	"sync"

	"github.com/equals-chan/ElectricHamster/internal/archive"
	"github.com/equals-chan/ElectricHamster/internal/config"
	"github.com/equals-chan/ElectricHamster/internal/job"
	"github.com/equals-chan/ElectricHamster/internal/store"
	"github.com/equals-chan/ElectricHamster/internal/vault"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Runtime holds the process-wide dependencies shared by the Wails services.
type Runtime struct {
	app         *application.App
	store       *store.Store
	archiver    archive.Archiver
	archiverErr error
	vaultPath   string

	mu      sync.Mutex
	vault   *vault.Vault
	cancel  context.CancelFunc
	running bool
	engine  *job.Engine
}

func newRuntime() (*Runtime, error) {
	appDB, err := config.DefaultAppDB()
	if err != nil {
		return nil, err
	}
	vaultPath, err := config.DefaultVault()
	if err != nil {
		return nil, err
	}
	st, err := store.Open(appDB)
	if err != nil {
		return nil, err
	}
	rt := &Runtime{store: st, vaultPath: vaultPath}
	// The archiver may be unavailable (no 7-Zip yet); surface the error lazily.
	rt.archiver, rt.archiverErr = archive.NewSevenZip(context.Background())
	return rt, nil
}

func (r *Runtime) Close() error {
	r.mu.Lock()
	if r.vault != nil {
		r.vault.Close()
		r.vault = nil
	}
	r.mu.Unlock()
	if r.store != nil {
		return r.store.Close()
	}
	return nil
}

func (r *Runtime) emit(name string, data any) {
	if r.app != nil {
		r.app.Event.Emit(name, data)
	}
}

// currentVault returns the open vault, or nil when none is open.
func (r *Runtime) currentVault() *vault.Vault {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.vault
}

func (r *Runtime) setVault(v *vault.Vault) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.vault = v
}

// beginRun returns false if a run is already in progress.
func (r *Runtime) beginRun(cancel context.CancelFunc) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return false
	}
	r.running = true
	r.cancel = cancel
	return true
}

func (r *Runtime) endRun() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
	r.cancel = nil
	r.engine = nil
}

func (r *Runtime) setEngine(e *job.Engine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.engine = e
}

func (r *Runtime) currentEngine() *job.Engine {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.engine
}

func (r *Runtime) stopRun() {
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
