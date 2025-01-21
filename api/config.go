package api

import "sync"

// AppConfig is the configured runtime information that's stored
// on disk and used to find and authenticate with the API.
type AppConfig interface {
	Name() string
	Env() DeploymentEnvironment
	AppKey() string
	AppSecret() string

	LoadCertificates() ([]byte, []byte, error)
	SaveCertificates(certPem, keyPem []byte) error
}

type appConfig struct {
	e    DeploymentEnvironment
	dir  string // stateDir
	lock sync.RWMutex
}

func NewAppConfig(envName, stateDir string) (AppConfig, error) {
	env, err := DeploymentEnvironmentFromString(envName)
	if err != nil {
		return nil, err
	}

	ac := &appConfig{e: env}

	err = ac.load()
	if err != nil {
		return nil, err
	}

	// todo: load data from api
	// reset vault information if name or appkey/secret changes?

	return ac, nil
}

func (a *appConfig) Name() string {
	return ""
}

func (a *appConfig) Env() DeploymentEnvironment {
	return a.e
}

func (a *appConfig) AppKey() string {
	return ""
}

func (a *appConfig) AppSecret() string {
	return ""
}
