package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	approle "github.com/hashicorp/vault/api/auth/approle"
	"go.ntppool.org/monitor/logger"
)

var vaultAddr = "https://vault.ntppool.org"

type Vault struct {
	key        string
	secret     string
	Token      string
	AuthSecret *vaultapi.Secret

	authPrefix string

	client *vaultapi.Client
	sync.RWMutex
}

func NewVault(key, secret, authPrefix string) (v *Vault, err error) {

	if key == "" || secret == "" || authPrefix == "" {
		return nil, fmt.Errorf("vault parameters required (key, secret, authPrefix)")
	}

	v = &Vault{
		key:        key,
		secret:     secret,
		authPrefix: authPrefix,
	}
	v.client, err = v.vaultClient()
	if err != nil {
		return nil, err
	}
	return v, err
}

func (v *Vault) Login(ctx context.Context) (*vaultapi.Secret, error) {

	if v.AuthSecret != nil && v.AuthSecret.Auth.Renewable && len(v.AuthSecret.Auth.ClientToken) > 0 {
		v.setAuthSecret(v.AuthSecret)

		auth, err := v.client.Auth().Token().RenewSelfWithContext(ctx, 120)
		if err != nil {
			var verr *vaultapi.ResponseError
			if errors.As(err, &verr) {
				if verr.StatusCode != 403 {
					logger.FromContext(ctx).Error("token renewal error", "err", err)
				}
			}
		} else {
			v.setAuthSecret(auth)
			return auth, nil
		}
	}

	roleID := v.key
	secretID := &approle.SecretID{FromString: v.secret}

	// log.Printf("RoleID: %s", roleID)
	appRoleAuth, err := approle.NewAppRoleAuth(
		roleID,
		secretID,
		approle.WithMountPath(fmt.Sprintf("%s/", v.authPrefix)),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to initialize AppRole auth method: %w", err)
	}

	v.client.SetToken("")

	// v.client.SetOutputCurlString(true)
	// _, err = v.client.Auth().Login(ctx, appRoleAuth)
	// cerr := &vaultapi.OutputStringError{}
	// if errors.As(err, &cerr) {
	// 	c, err := cerr.CurlString()
	// 	if err != nil {
	// 		log.Fatalf("no curl string: %s", err)
	// 	}
	// 	log.Printf("curl: %s", c)
	// }
	// v.client.SetOutputCurlString(false)

	authInfo, err := v.client.Auth().Login(ctx, appRoleAuth)

	if err != nil {
		var verr *vaultapi.ResponseError
		if errors.As(err, &verr) {
			if verr.StatusCode == 400 {
				return nil, AuthenticationError{Message: "invalid api key or api secret"}
			}
		}
		return nil, fmt.Errorf("unable to login to AppRole: %w", err)
	}
	if authInfo == nil {
		return nil, fmt.Errorf("no auth info was returned after login")
	}

	// log.Printf("Authenticated API key")
	// log.Printf("authInfo: %+v", authInfo)
	// log.Printf("Token: %s", authInfo.Auth.ClientToken)

	v.setAuthSecret(authInfo)

	return authInfo, nil
}

func (v *Vault) setAuthSecret(secret *vaultapi.Secret) error {

	err := func() error {
		v.Lock()
		defer v.Unlock()

		v.Token = secret.Auth.ClientToken
		v.AuthSecret = secret

		v.client.SetToken(v.Token)

		return nil
	}()
	if err != nil {
		return err
	}

	return nil
}

func (cr *Vault) vaultClient() (*vaultapi.Client, error) {

	if cr.client != nil {
		return cr.client, nil
	}

	vaultConfig := &vaultapi.Config{
		Address: vaultAddr,
	}

	client, err := vaultapi.NewClient(vaultConfig)
	if err != nil {
		return nil, err
	}

	cr.client = client

	return client, nil
}

// func (cr *Vault) checkToken(ctx context.Context) (bool, *vaultapi.Secret, error) {
// 	client, err := cr.vaultClient()
// 	if err != nil {
// 		return false, nil, err
// 	}

// 	rv, err := client.Logical().ReadWithContext(ctx, "auth/token/lookup-self")
// 	if err != nil {
// 		return false, nil, err
// 	}

// 	log.Printf("Session token expires: %s, remaining uses: %s", rv.Data["expire_time"], rv.Data["num_uses"])

// 	return true, rv, nil
// }

func (cr *Vault) SecretInfo(ctx context.Context, name string) (map[string]interface{}, error) {
	client, err := cr.vaultClient()
	if err != nil {
		return nil, err
	}

	rv, err := client.Logical().WriteWithContext(ctx,
		fmt.Sprintf("auth/%s/role/%s/secret-id/lookup",
			cr.authPrefix,
			name,
		),
		map[string]interface{}{
			"secret_id": cr.secret,
		},
	)

	if err != nil {
		return nil, err
	}

	if rv == nil {
		return nil, fmt.Errorf("no secret found")
	}

	return rv.Data, nil
}

func (cr *Vault) RenewToken(ctx context.Context, authInfo *vaultapi.Secret, updateChannel chan<- bool) error {

	log := logger.FromContext(ctx)
	// log.Printf("starting RenewToken, with authInfo: %+v", authInfo)

	for {
		var err error
		if authInfo == nil {
			authInfo, err = cr.Login(ctx)
			if err != nil {
				log.Error("unable to authenticate to Vault", "err", err)
				return err
			}
		}
		tokenErr := cr.manageTokenLifecycle(ctx, authInfo, updateChannel)
		if tokenErr != nil {
			log.Error("unable to start managing token lifecycle", "err", tokenErr)
			return err

		}

		if err = ctx.Err(); err != nil {
			return err
		}

		authInfo = nil
	}
}

// Starts token lifecycle management. Returns only fatal errors as errors,
// otherwise returns nil so we can attempt login again.
func (cr *Vault) manageTokenLifecycle(ctx context.Context, token *vaultapi.Secret, updateChannel chan<- bool) error {

	log := logger.FromContext(ctx)

	renew := token.Auth.Renewable // You may notice a different top-level field called Renewable. That one is used for dynamic secrets renewal, not token renewal.
	if !renew {
		log.Info("Token is not configured to be renewable. Re-attempting login.")
		return nil
	}

	watcher, err := cr.client.NewLifetimeWatcher(&vaultapi.LifetimeWatcherInput{
		Secret: token,
		// Increment: 3600, // Learn more about this optional value in https://www.vaultproject.io/docs/concepts/lease#lease-durations-and-renewal
	})
	if err != nil {
		return fmt.Errorf("unable to initialize new lifetime watcher for renewing auth token: %w", err)
	}

	go watcher.Start()
	defer watcher.Stop()

	for {
		select {
		// `DoneCh` will return if renewal fails, or if the remaining lease
		// duration is under a built-in threshold and either renewing is not
		// extending it or renewing is disabled. In any case, the caller
		// needs to attempt to log in again.
		case err := <-watcher.DoneCh():
			if err != nil {
				log.Warn("failed to renew token. Re-attempting login.", "err", err)
				return nil
			}
			if err = ctx.Err(); err != nil {
				return nil
			}
			// This occurs once the token has reached max TTL.
			log.Warn("token can no longer be renewed. Re-attempting login.")
			return nil

		// Successfully completed renewal
		case renewal := <-watcher.RenewCh():
			updateChannel <- true
			log.Info("successfully renewed token")
			// js, err := json.MarshalIndent(renewal, "", "  ")
			cr.setAuthSecret(renewal.Secret)

		case <-ctx.Done():
			watcher.Stop()
			time.Sleep(100 * time.Millisecond) // allow the watcher to mark itself done
		}
	}
}

// Type alias for the recursive call marshaling JSON with a lock
type jVault Vault

func (v *Vault) MarshalJSON() ([]byte, error) {
	v.RLock()
	defer v.RUnlock()

	return json.Marshal(jVault{
		Token:      v.Token,
		AuthSecret: v.AuthSecret,
	})
}
