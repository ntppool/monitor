package vault

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strconv"
	"sync"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	"inet.af/netaddr"
)

// On the server we use vault-agent to authenticate and
// manage the token, so all that code from the client
// goes away.

type notFoundError struct{}

func (m *notFoundError) Error() string {
	return "token not found"
}

type token struct {
	Secret  string `json:"token"`
	Created int64  `json:"token-ts"`
	version int    `json:"-"`
}

type TokenManager struct {
	key      string
	basePath string

	latest   *token
	versions map[int]*token

	vault *vaultapi.Client
	lock  sync.RWMutex
}

func New(key, depEnv string) (*TokenManager, error) {

	if len(depEnv) == 0 {
		return nil, fmt.Errorf("invalid deployment mode parameter %q", depEnv)
	}

	var basePath = fmt.Sprintf("kv/data/ntppool/%s/", depEnv)

	cl, err := vaultClient()
	if err != nil {
		return nil, err
	}

	tm := &TokenManager{
		key:      key,
		basePath: basePath,
		vault:    cl,
		versions: map[int]*token{},
	}

	err = tm.populate()
	if err != nil {
		return nil, err
	}

	// todo: pass context so it can be shutdown
	go tm.rotateTokensBackground()

	return tm, nil
}

func getSignatureVersion(sig []byte) (int, error) {
	idx := bytes.IndexByte(sig, '-')
	if idx < 1 {
		return 0, fmt.Errorf("invalid signature")
	}

	versionb := sig[0:idx]
	version, err := strconv.Atoi(string(versionb))
	if err != nil || version == 0 {
		return 0, fmt.Errorf("unknown signature version %d: %s", version, err)
	}
	return version, nil
}

func (tm *TokenManager) Validate(monitorID int32, batchID []byte, ip *netaddr.IP, sig []byte) (bool, error) {
	version, err := getSignatureVersion(sig)
	if err != nil || version == 0 {
		return false, err
	}

	token, err := tm.getTokenVersion(context.Background(), version)
	if err != nil {
		return false, err
	}

	expected, err := tm.signWith(monitorID, batchID, ip, token)
	if err != nil {
		return false, err
	}
	if len(expected) > 0 && bytes.Compare(sig, expected) == 0 {
		return true, nil
	}

	log.Printf("exp: %s", expected)
	log.Printf("got: %s", sig)

	return false, fmt.Errorf("could not validate signature")
}

func (tm *TokenManager) Sign(monitorID int32, batchID []byte, ip *netaddr.IP) ([]byte, error) {
	token, err := tm.getToken(context.Background())
	if err != nil {
		return nil, err
	}
	// log.Printf("got version: %d, token: %s", token.version, token.Secret)

	return tm.signWith(monitorID, batchID, ip, token)
}

func (tm *TokenManager) signWith(monitorID int32, batchID []byte, ip *netaddr.IP, token *token) ([]byte, error) {

	hm := hmac.New(sha256.New, []byte(token.Secret))

	monIDb := strconv.AppendInt([]byte{}, int64(monitorID), 10)

	ipb, err := ip.MarshalBinary()
	if err != nil {
		return nil, err
	}

	data := bytes.Join([][]byte{
		[]byte(batchID),
		monIDb,
		ipb,
	}, []byte("|"))

	p, err := hm.Write([]byte(data))
	if err != nil || p != len(data) {
		return nil, fmt.Errorf("hmac error: %s", err)
	}

	sha := hm.Sum(nil)

	r := strconv.AppendInt([]byte{}, int64(token.version), 10)

	r = append(r, []byte("-")...)

	shaenc := make([]byte, base64.RawURLEncoding.EncodedLen(len(sha)))
	base64.RawURLEncoding.Encode(shaenc, sha)

	r = append(r, shaenc...)

	return r, nil
}

func (tm *TokenManager) rotateTokensBackground() {
	ctx := context.Background() // for when the app has context properly

	l := log.New(os.Stderr, "rotateTokensBackground: ", 0)

	ticker := time.NewTicker(2 * time.Hour)
	defer ticker.Stop()

	for {

		latest, err := tm.getToken(ctx)
		if err != nil {
			l.Printf("could not get token: %s", err)
		}

		if age := (time.Now().Unix() - int64(latest.Created)); age > 28800 {
			l.Printf("latest token is more than eight hours old (%d seconds), rotate it", age)
			tm.createNewToken(ctx, latest.version)
			tm.lock.Lock()
			tm.latest = nil
			tm.lock.Unlock()
			tm.getToken(ctx)

		}

		select {
		case <-ctx.Done():
			l.Printf("context done")
			return
		case <-ticker.C:
		}

	}
}

func (tm *TokenManager) createNewToken(ctx context.Context, cas int) error {
	data := map[string]interface{}{
		"data": makeToken(),
		"metadata": map[string]interface{}{
			"cas_required": true,
		},
		"options": map[string]interface{}{
			"cas": cas,
		},
	}

	_, err := tm.vault.Logical().WriteWithContext(ctx, tm.path(), data)
	if err != nil {
		return err
	}

	return nil
}

func (tm *TokenManager) path() string {
	return path.Join(tm.basePath, tm.key)
}

func (tm *TokenManager) populate() error {
	ctx := context.Background()

	t, err := tm.getToken(ctx)
	if err != nil {
		if _, ok := err.(*notFoundError); !ok {
			return err
		}
	}

	if t == nil {
		err := tm.createNewToken(ctx, 0)
		if err != nil {
			return fmt.Errorf("could not save token: %s", err)
		}

		t, err = tm.getToken(ctx)
		if err != nil {
			return err
		}
		if t == nil {
			return fmt.Errorf("could not find token data")
		}
	}

	if t != nil {
		tm.latest = t
		tm.versions[t.version] = t
	}

	return nil
}

func (tm *TokenManager) getTokenVersionCache(ctx context.Context, version int) (*token, error) {
	tm.lock.RLock()
	defer tm.lock.RUnlock()

	if t, ok := tm.versions[version]; ok {
		return t, nil
	}

	latest, err := tm.getToken(ctx)
	if err != nil {
		return nil, err
	}

	if latest.version < version {
		return nil, fmt.Errorf("invalid signature version")
	}

	return nil, nil
}

func (tm *TokenManager) getTokenVersion(ctx context.Context, version int) (*token, error) {

	token, err := tm.getTokenVersionCache(ctx, version)
	if err != nil {
		return nil, err
	}
	if token != nil {
		return token, nil
	}

	tm.lock.Lock()
	defer tm.lock.Unlock()

	// in case it was set while we were waiting for a lock
	if token, ok := tm.versions[version]; ok {
		return token, nil
	}

	log.Printf("requesting token %q/%d", tm.key, version)
	rv, err := tm.getKVVersion(ctx, tm.key, version)
	if err != nil {
		return nil, err
	}

	token, err = parseTokenVaultSecret(rv.Data)
	if err != nil {
		return nil, err
	}

	return token, err

}

func (tm *TokenManager) getToken(ctx context.Context) (*token, error) {
	tm.lock.RLock()

	if token := tm.latest; token != nil {
		tm.lock.RUnlock()
		return token, nil
	}

	tm.lock.RUnlock()
	tm.lock.Lock()
	defer tm.lock.Unlock()

	// in case it was set while we were waiting for a lock
	if tm.latest != nil {
		return tm.latest, nil
	}

	rv, err := tm.getKV(ctx, tm.key)
	if err != nil {
		return nil, err
	}
	if rv == nil {
		return nil, &notFoundError{}
	}

	t, err := parseTokenVaultSecret(rv.Data)
	if err != nil {
		return nil, err
	}

	return t, nil
}

func parseTokenVaultSecret(data map[string]interface{}) (*token, error) {
	t := &token{}

	var err error

	if dataif, ok := data["data"]; ok {
		data := dataif.(map[string]interface{})

		if tokData, ok := data["token"]; ok {
			if tokStr, ok := tokData.(string); ok {
				t.Secret = tokStr
			}
		}

		if tokData, ok := data["token-ts"]; ok {
			if tokInt, ok := tokData.(json.Number); ok {
				t.Created, err = tokInt.Int64()
				if t.Created == 0 || err != nil {
					log.Printf("could not parse Created from token secret (%T: %+v): %s", tokData, tokData, err)
				}
			}
		}
	}

	if metaif, ok := data["metadata"]; ok {
		meta := metaif.(map[string]interface{})
		if version, ok := meta["version"]; ok {
			if v, ok := version.(json.Number); ok {
				v64, err := v.Int64()
				if err != nil {
					return nil, err
				}
				t.version = int(v64)
			}
		}
	}

	if t.version == 0 || len(t.Secret) == 0 {
		return nil, fmt.Errorf("expected token data not found")
	}

	return t, nil
}

func makeToken() *token {
	randomBytes := make([]byte, 16)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return nil
	}
	return &token{
		Secret:  base64.URLEncoding.EncodeToString(randomBytes),
		Created: time.Now().Unix(),
	}
}

var hasOutputVaultEnvMessage bool

func vaultClient() (*vaultapi.Client, error) {

	c := vaultapi.DefaultConfig()

	if c.Address == "https://127.0.0.1:8200" {
		c.Address = "https://vault.ntppool.org"
	}

	cl, err := vaultapi.NewClient(c)
	if err != nil {
		return nil, err
	}

	// VAULT_TOKEN is read automatically from the environment if set
	// so we just try the file here
	token, err := ioutil.ReadFile("/vault/secrets/token")
	if err == nil {
		cl.SetToken(string(token))
	} else {
		if !hasOutputVaultEnvMessage {
			hasOutputVaultEnvMessage = true
			log.Printf("could not read /vault/secrets/token (%s), using VAULT_TOKEN", err)
		}
	}

	return cl, nil
}

func (tm *TokenManager) getKV(ctx context.Context, k string) (*vaultapi.Secret, error) {

	cl, err := vaultClient()
	if err != nil {
		return nil, nil
	}

	rv, err := cl.Logical().ReadWithContext(ctx, tm.path())
	if err != nil {
		return nil, err
	}

	return rv, nil
}

func (tm *TokenManager) getKVVersion(ctx context.Context, k string, version int) (*vaultapi.Secret, error) {

	cl, err := vaultClient()
	if err != nil {
		return nil, nil
	}

	data := map[string][]string{
		"version": {strconv.Itoa(version)},
	}

	rv, err := cl.Logical().ReadWithDataWithContext(ctx, tm.path(), data)
	if err != nil {
		return nil, err
	}

	return rv, nil
}

func (tm *TokenManager) SetKV(ctx context.Context, k string, data *vaultapi.Secret) error {
	p := tm.path()
	cl, err := vaultClient()
	if err != nil {
		return nil
	}

	_, err = cl.Logical().WriteWithContext(ctx, p, data.Data)
	if err != nil {
		return err
	}

	return nil
}
