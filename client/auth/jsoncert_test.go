package auth

// import (
// 	"crypto/rsa"
// 	"crypto/x509"
// 	"encoding/json"
// 	"fmt"
// 	"math/big"
// 	"testing"
// 	"time"

// 	"crypto/rand"
// )

// func TestJSONCerts(t *testing.T) {

// 	testKey, _ := rsa.GenerateKey(rand.Reader, 1024)

// 	tmpl := &x509.Certificate{
// 		NotBefore:    time.Now().Add(-1 * time.Minute),
// 		NotAfter:     time.Now().Add(12 * time.Hour),
// 		DNSNames:     []string{"test.example.com"},
// 		SerialNumber: big.NewInt(42),
// 	}

// 	certDer, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &testKey.PublicKey, testKey)
// 	if err != nil {
// 		t.Log(err)
// 		t.FailNow()
// 	}

// 	if len(certDer) == 0 {
// 		t.Log("empty certificate")
// 		t.Fail()
// 	}

// 	cert, err := x509.ParseCertificate(certDer)
// 	if err != nil {
// 		t.Log(err)
// 		t.FailNow()
// 	}

// 	if cert.SerialNumber.Cmp(big.NewInt(42)) != 0 {
// 		t.Logf("unexpected serial number %d", cert.SerialNumber)
// 	}

// 	st := ClientAuth{
// 		Cert: cert,
// 	}

// 	// ioutil.WriteFile("/tmp/cert.der", cert.Raw, 0644)

// 	js, err := json.Marshal(&st)
// 	if err != nil {
// 		t.Log(err)
// 		t.FailNow()
// 	}
// 	t.Log(fmt.Sprintf("json: %s", js))

// 	// ioutil.WriteFile("/tmp/cert.json", js, 0644)

// }
