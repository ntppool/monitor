package config

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"

	"go.ntppool.org/common/logger"
)

const stateFile = "state.json"

func (ac *appConfig) SaveCertificates(ctx context.Context, certPem, keyPem []byte) error {
	log := logger.FromContext(ctx)
	err := replaceFile(ac.stateFilePrefix("cert.pem"), certPem)
	if err != nil {
		log.Error("Failed to save cert.pem", "err", err)
		return err
	}
	log.DebugContext(ctx, "Saved cert.pem", "length", len(certPem))

	err = replaceFile(ac.stateFilePrefix("key.pem"), keyPem)
	if err != nil {
		log.Error("Failed to save key.pem", "err", err)
		return err
	}
	log.DebugContext(ctx, "Saved key.pem", "length", len(keyPem))

	return nil
}

func (ac *appConfig) LoadCertificates(ctx context.Context) error {
	log := logger.FromContext(ctx)
	certPem, err := os.ReadFile(ac.stateFilePrefix("cert.pem"))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.ErrorContext(ctx, "Failed to read cert.pem", "err", err)
		}
		return err
	}
	log.DebugContext(ctx, "Loaded cert.pem", "length", len(certPem))
	keyPem, err := os.ReadFile(ac.stateFilePrefix("key.pem"))
	if err != nil {
		log.ErrorContext(ctx, "Failed to read key.pem", "err", err)
		return err
	}
	log.DebugContext(ctx, "Loaded key.pem", "length", len(keyPem))

	tlsCert, err := tls.X509KeyPair(certPem, keyPem)
	if err != nil {
		log.ErrorContext(ctx, "Failed to parse X509KeyPair", "err", err)
		return err
	}
	log.DebugContext(ctx, "Parsed X509KeyPair successfully")

	return ac.setCertificate(ctx, &tlsCert)
}

func (ac *appConfig) stateFilePrefix(filename string) string {
	// spew.Dump("ac", ac)

	dir := path.Join(ac.dir, ac.Env().String())

	_, err := os.Stat(dir)
	if err != nil {
		os.MkdirAll(dir, 0o700)
	}

	return path.Join(dir, filename)
}

func (ac *appConfig) load(ctx context.Context) error {
	log := logger.FromContext(ctx)

	// Capture previous state for change detection
	ac.lock.RLock()
	prevAPIKey := ac.API.APIKey
	prevHaveCert := ac.tlsCert != nil
	ac.lock.RUnlock()

	// Check if state file exists before loading
	stateFilePath := ac.stateFilePrefix(stateFile)
	_, err := os.Stat(stateFilePath)
	stateFileExisted := err == nil

	err = ac.loadFromDisk(ctx)
	if err != nil {
		return err
	}

	err = ac.LoadCertificates(ctx)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.InfoContext(ctx, "load certificate", "err", err)
		}
	}

	// Check API key without holding lock to prevent deadlocks during HTTP calls
	ac.lock.RLock()
	haveAPIKey := ac.API.APIKey != ""
	currentAPIKey := ac.API.APIKey
	currentHaveCert := ac.tlsCert != nil
	ac.lock.RUnlock()

	// Check if API key or certificate status changed
	configChanged := prevAPIKey != currentAPIKey || prevHaveCert != currentHaveCert

	// Notify if local changes were detected (API key or certificate changes)
	// Do this before API calls so notifications happen even if API fails
	if configChanged {
		log.InfoContext(ctx, "local config changed",
			"api_key_changed", prevAPIKey != currentAPIKey,
			"cert_changed", prevHaveCert != currentHaveCert)
		ac.notifyConfigChange()
	}

	var apiDataChanged bool
	if haveAPIKey {
		apiDataChanged, err = ac.LoadAPIAppConfig(ctx)
		if err != nil {
			return err
		}
	}

	// Only save if:
	// 1. State file didn't exist (first run), OR
	// 2. API key changed, OR
	// 3. API data changed
	shouldSave := !stateFileExisted || configChanged || apiDataChanged
	if shouldSave {
		return ac.save()
	}
	return nil
}

func (ac *appConfig) loadFromDisk(ctx context.Context) error {
	log := logger.FromContext(ctx)
	ac.lock.Lock()
	defer ac.lock.Unlock()

	path := ac.stateFilePrefix(stateFile)
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Try to migrate from old runtime directory location
			if err := ac.tryMigrateFromRuntimeDir(ctx, path); err != nil {
				log.DebugContext(ctx, "migration from runtime directory failed", "err", err)
			}
			// File doesn't exist (and migration failed or wasn't needed), save will be handled by caller
			return nil
		} else {
			return err
		}
	}

	err = json.Unmarshal(b, &ac)
	if err != nil {
		return err
	}

	return nil
}

func (ac *appConfig) save() error {
	ac.lock.Lock()
	defer ac.lock.Unlock()

	path := ac.stateFilePrefix(stateFile)

	b, err := json.MarshalIndent(ac, "", "  ")
	if err != nil {
		return err
	}
	err = replaceFile(path, b)
	if err != nil {
		return err
	}

	return nil
}

// tryMigrateFromRuntimeDir attempts to migrate state.json from $RUNTIME_DIRECTORY to new location
func (ac *appConfig) tryMigrateFromRuntimeDir(ctx context.Context, newPath string) error {
	log := logger.FromContext(ctx)

	// Check if RUNTIME_DIRECTORY is set
	runtimeDir := os.Getenv("RUNTIME_DIRECTORY")
	if runtimeDir == "" {
		// No runtime directory, nothing to migrate
		return nil
	}

	// Build old state file path
	oldPath := path.Join(runtimeDir, ac.Env().String(), stateFile)

	// Try to read old state file
	oldData, err := os.ReadFile(oldPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.DebugContext(ctx, "no old state file to migrate", "path", oldPath)
			return nil
		}
		return err
	}

	log.InfoContext(ctx, "migrating state from runtime directory", "from", oldPath, "to", newPath)

	// Ensure new directory exists
	if err := os.MkdirAll(path.Dir(newPath), 0o700); err != nil {
		return err
	}

	// Write to new location
	if err := replaceFile(newPath, oldData); err != nil {
		return err
	}

	// Load the migrated data into current instance
	if err := json.Unmarshal(oldData, ac); err != nil {
		return err
	}

	// Also migrate certificate files if they exist
	ac.migrateCertificates(ctx, runtimeDir)

	log.InfoContext(ctx, "successfully migrated state from runtime directory")
	return nil
}

// migrateCertificates migrates certificate files from runtime directory
func (ac *appConfig) migrateCertificates(ctx context.Context, runtimeDir string) {
	log := logger.FromContext(ctx)

	oldCertDir := path.Join(runtimeDir, ac.Env().String())

	for _, filename := range []string{"cert.pem", "key.pem"} {
		oldPath := path.Join(oldCertDir, filename)
		newPath := ac.stateFilePrefix(filename)

		data, err := os.ReadFile(oldPath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				log.DebugContext(ctx, "failed to read old certificate file", "path", oldPath, "err", err)
			}
			continue
		}

		if err := replaceFile(newPath, data); err != nil {
			log.WarnContext(ctx, "failed to migrate certificate file", "from", oldPath, "to", newPath, "err", err)
		} else {
			log.InfoContext(ctx, "migrated certificate file", "from", oldPath, "to", newPath)
		}
	}
}

func replaceFile(path string, b []byte) error {
	tmpPath := path + ".tmp"

	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer func() {
		f.Close()
		if err != nil {
			os.Remove(tmpPath)
		}
	}()

	n, err := f.Write(b)
	if err == nil && n < len(b) {
		err = io.ErrShortWrite
	}
	if err1 := f.Close(); err == nil {
		err = err1
	}
	if err != nil {
		return err
	}

	// Set restrictive permissions for private key files
	if len(path) >= 7 && path[len(path)-7:] == "key.pem" {
		err = os.Chmod(tmpPath, 0o600)
		if err != nil {
			return err
		}
	}

	err = os.Rename(tmpPath, path)
	if err != nil {
		return err
	}

	return nil
}
