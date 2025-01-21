package api

import (
	"encoding/json"
	"io"
	"os"
	"path"
)

const stateFile = "state.json"

func (ac *appConfig) SaveCertificates(certPem, keyPem []byte) error {
	err := replaceFile(ac.stateFilePrefix("cert.pem"), certPem)
	if err != nil {
		return err
	}

	err = replaceFile(ac.stateFilePrefix("key.pem"), keyPem)
	if err != nil {
		return err
	}

	return nil
}

func (ac *appConfig) LoadCertificates() ([]byte, []byte, error) {
	certPem, err := os.ReadFile(ac.stateFilePrefix("cert.pem"))
	if err != nil {
		return nil, nil, err
	}
	keyPem, err := os.ReadFile(ac.stateFilePrefix("key.pem"))
	if err != nil {
		return nil, nil, err
	}
	return certPem, keyPem, nil
}

func (ac *appConfig) stateFilePrefix(filename string) string {
	dir := path.Join(ac.dir, ac.Name())

	_, err := os.Stat(dir)
	if err != nil {
		os.MkdirAll(dir, 0o700)
	}

	return path.Join(dir, filename)
}

func (ac *appConfig) load() error {
	path := ac.stateFilePrefix(stateFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return err
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
