package auth

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"os"
	"path"
)

const stateFile = "state.json"

func (ca *ClientAuth) stateFilePrefix(filename string) string {
	dir := path.Join(ca.dir, ca.Name)

	_, err := os.Stat(dir)
	if err != nil {
		os.MkdirAll(dir, 0700)
	}

	return path.Join(dir, filename)
}

func (ca *ClientAuth) load() error {
	path := ca.stateFilePrefix(stateFile)
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	ca.lock.Lock()
	defer ca.lock.Unlock()

	err = json.Unmarshal(b, &ca)
	if err != nil {
		return err
	}

	return nil
}

func (ca *ClientAuth) save() error {

	path := ca.stateFilePrefix(stateFile)

	var err error

	ca.lock.RLock()
	defer ca.lock.RUnlock()

	b, err := json.MarshalIndent(ca, "", "  ")
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
