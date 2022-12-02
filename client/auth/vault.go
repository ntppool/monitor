package auth

import (
	"context"
	"fmt"
	"log"
	"sync"

	vaultapi "github.com/hashicorp/vault/api"
	approle "github.com/hashicorp/vault/api/auth/approle"
)

var vaultAddr = "https://vault.ntppool.org"

const authPrefix = "/monitors"

type Vault struct {
	key    string
	secret string
	Token  string

	client *vaultapi.Client
	lock   sync.RWMutex

	deploymentEnvironment string
}

func (v *Vault) Login(ctx context.Context, depEnv string) error {

	v.deploymentEnvironment = depEnv

	// setToken sets up the client if it's not already
	v.setToken(v.Token)
	ok, err := v.checkToken(ctx)
	if ok {
		return nil
	}
	if verr, ok := err.(*vaultapi.ResponseError); ok {
		if verr.StatusCode != 403 {
			log.Printf("token lookup error: %s", err)
		}
	}

	roleID := v.key
	secretID := &approle.SecretID{FromString: v.secret}

	// log.Printf("RoleID: %s", roleID)
	appRoleAuth, err := approle.NewAppRoleAuth(
		roleID,
		secretID,
		approle.WithMountPath(fmt.Sprintf("%s/%s/", authPrefix, depEnv)),
	)
	if err != nil {
		return fmt.Errorf("unable to initialize AppRole auth method: %w", err)
	}

	authInfo, err := v.client.Auth().Login(context.TODO(), appRoleAuth)
	if err != nil {
		return fmt.Errorf("unable to login to AppRole auth method: %w", err)
	}
	if authInfo == nil {
		return fmt.Errorf("no auth info was returned after login")
	}

	log.Printf("Authenticated API key")
	// log.Printf("authInfo: %+v", authInfo)
	// log.Printf("Token: %s", authInfo.Auth.ClientToken)

	v.setToken(authInfo.Auth.ClientToken)

	return nil
}

func (v *Vault) setToken(token string) error {

	var oldToken string

	err := func() error {
		v.lock.Lock()
		defer v.lock.Unlock()

		var err error

		oldToken = v.Token
		v.Token = token

		if v.client == nil {
			v.client, err = v.vaultClient()
			if err != nil {
				return err
			}
		}
		v.client.SetToken(token)

		return nil
	}()
	if err != nil {
		return err
	}

	if token == oldToken {
		return nil
	}

	return nil
}

func (cr *Vault) vaultClient() (*vaultapi.Client, error) {

	if cr.client != nil {
		return cr.client, nil
	}

	vaultConfig := &vaultapi.Config{
		Address:   vaultAddr,
		SRVLookup: true,
	}

	client, err := vaultapi.NewClient(vaultConfig)
	if err != nil {
		return nil, err
	}

	cr.client = client

	return client, nil
}

func (cr *Vault) checkToken(ctx context.Context) (bool, error) {
	client, err := cr.vaultClient()
	if err != nil {
		return false, err
	}

	rv, err := client.Logical().ReadWithContext(ctx, "auth/token/lookup-self")
	if err != nil {
		return false, err
	}

	log.Printf("Session token expires: %s, remaining uses: %s", rv.Data["expire_time"], rv.Data["num_uses"])

	return true, nil
}

func (cr *Vault) SecretInfo(ctx context.Context, name string) (map[string]interface{}, error) {
	client, err := cr.vaultClient()
	if err != nil {
		return nil, err
	}

	rv, err := client.Logical().WriteWithContext(ctx,
		fmt.Sprintf("auth/%s/%s/role/%s/secret-id/lookup",
			authPrefix,
			cr.deploymentEnvironment,
			name,
		),
		map[string]interface{}{
			"secret_id": cr.secret,
		},
	)

	if err != nil {
		return nil, err
	}

	return rv.Data, nil
}
