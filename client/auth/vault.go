package auth

import (
	"context"
	"fmt"
	"log"

	vaultapi "github.com/hashicorp/vault/api"
	approle "github.com/hashicorp/vault/api/auth/approle"
)

var vaultAddr = "https://vault.ntppool.org"
var vaultMount = "monitors/devel/"

func (v *Vault) Login(ctx context.Context) error {

	client, err := v.vaultClient()
	if err != nil {
		return err
	}

	if len(v.Token) > 0 {
		ok, err := v.checkToken(ctx)
		if ok {
			return nil
		}
		log.Printf("token error: %s", err)
	}

	roleID := v.key
	secretID := &approle.SecretID{FromString: v.secret}

	log.Printf("RoleID: %s", roleID)
	appRoleAuth, err := approle.NewAppRoleAuth(
		roleID,
		secretID,
		approle.WithMountPath("/monitors/devel/"),
	)
	if err != nil {
		return fmt.Errorf("unable to initialize AppRole auth method: %w", err)
	}

	authInfo, err := client.Auth().Login(context.TODO(), appRoleAuth)
	if err != nil {
		return fmt.Errorf("unable to login to AppRole auth method: %w", err)
	}
	if authInfo == nil {
		return fmt.Errorf("no auth info was returned after login")
	}

	log.Printf("authInfo: %+v", authInfo)
	log.Printf("Token: %s", authInfo.Auth.ClientToken)

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

	rv, err := client.Logical().ReadWithContext(ctx, "/auth/token/lookup-self")
	if err != nil {
		return false, err
	}

	log.Printf("token self data: %+v", rv)

	return true, nil
}
