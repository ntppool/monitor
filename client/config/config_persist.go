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

	err := ac.loadFromDisk(ctx)
	if err != nil {
		return err
	}

	err = ac.LoadCertificates(ctx)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.InfoContext(ctx, "load certificate", "err", err)
		}
	}

	haveAPIKey := ac.API.APIKey != ""
	// log.DebugContext(ctx, "loaded configuration from disk", "name", ac.Name(), "api_key", haveAPIKey)

	if haveAPIKey {
		err = ac.LoadAPIAppConfig(ctx)
		if err != nil {
			return err
		}
	}

	// todo: check if it changed?
	return ac.save()
}

func (ac *appConfig) loadFromDisk(_ context.Context) error {
	path := ac.stateFilePrefix(stateFile)
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ac.save()
		} else {
			return err
		}
	}

	ac.lock.Lock()
	defer ac.lock.Unlock()

	err = json.Unmarshal(b, &ac)
	if err != nil {
		return err
	}

	return nil
}

func (ac *appConfig) save() error {
	path := ac.stateFilePrefix(stateFile)

	var err error

	ac.lock.RLock()
	defer ac.lock.RUnlock()

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

	err = os.Rename(tmpPath, path)
	if err != nil {
		return err
	}

	return nil
}
