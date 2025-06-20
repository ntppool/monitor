package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.ntppool.org/common/config/depenv"
	apitls "go.ntppool.org/monitor/api/tls"
)

// generateTestCertificate creates a test certificate and private key
func generateTestCertificate(t *testing.T, notBefore, notAfter time.Time) ([]byte, []byte) {
	t.Helper()

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"Test Organization"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{"Test City"},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		KeyUsage:    x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: nil,
	}

	// Generate certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	require.NoError(t, err)

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	keyDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyDER,
	})

	return certPEM, keyPEM
}

func TestCertificateLoadSave(t *testing.T) {
	t.Run("valid certificate persistence", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Generate test certificate
		notBefore := time.Now().Add(-time.Hour)
		notAfter := time.Now().Add(24 * time.Hour)
		certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter)

		// Save certificates
		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
		require.NoError(t, err)

		// Verify files were created
		certPath := filepath.Join(env.tmpDir, depenv.DeployDevel.String(), "cert.pem")
		keyPath := filepath.Join(env.tmpDir, depenv.DeployDevel.String(), "key.pem")

		_, err = os.Stat(certPath)
		assert.NoError(t, err, "cert.pem should exist")

		_, err = os.Stat(keyPath)
		assert.NoError(t, err, "key.pem should exist")

		// Load certificates
		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		assert.NoError(t, err, "should load certificates successfully")

		// Verify certificate is available
		assert.True(t, env.cfg.HaveCertificate(), "should have certificate after loading")
	})

	t.Run("malformed certificate handling", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Save invalid certificate data
		invalidCert := []byte("invalid cert data")
		invalidKey := []byte("invalid key data")

		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, invalidCert, invalidKey)
		require.NoError(t, err) // Saving should work

		// Loading should fail
		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		assert.Error(t, err, "should fail to load invalid certificates")

		assert.False(t, env.cfg.HaveCertificate(), "should not have certificate after failed load")
	})

	t.Run("missing file scenarios", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Try to load non-existent certificates
		err := env.cfg.(*appConfig).LoadCertificates(env.ctx)
		assert.Error(t, err, "should fail when certificate files don't exist")
		assert.True(t, os.IsNotExist(err), "should be a file not found error")

		assert.False(t, env.cfg.HaveCertificate(), "should not have certificate")
	})

	t.Run("concurrent certificate updates", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping concurrent test in short mode")
		}

		// Generate test certificates
		notBefore := time.Now().Add(-time.Hour)
		notAfter := time.Now().Add(24 * time.Hour)

		var allErrors []error
		var errorMutex sync.Mutex

		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				// Each goroutine gets its own config instance with separate directory
				env, cleanup := setupTestConfig(t)
				defer cleanup()

				for j := 0; j < 3; j++ {
					// Generate unique certificate for this goroutine
					certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter)

					// Save certificates
					err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
					if err != nil {
						errorMutex.Lock()
						allErrors = append(allErrors, err)
						errorMutex.Unlock()
						continue
					}

					// Load certificates
					err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
					if err != nil {
						errorMutex.Lock()
						allErrors = append(allErrors, err)
						errorMutex.Unlock()
					}

					time.Sleep(time.Millisecond) // Small delay
				}
			}(i)
		}

		wg.Wait()

		// Check for errors
		errorMutex.Lock()
		for _, err := range allErrors {
			t.Errorf("Concurrent certificate operation error: %v", err)
		}
		errorMutex.Unlock()
	})

	t.Run("partial file corruption recovery", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Generate and save valid certificates
		notBefore := time.Now().Add(-time.Hour)
		notAfter := time.Now().Add(24 * time.Hour)
		certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter)

		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
		require.NoError(t, err)

		// Corrupt the key file
		keyPath := filepath.Join(env.tmpDir, depenv.DeployDevel.String(), "key.pem")
		err = os.WriteFile(keyPath, []byte("corrupted key"), 0o600)
		require.NoError(t, err)

		// Loading should fail gracefully
		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		assert.Error(t, err, "should fail with corrupted key file")
	})
}

func TestCertificateValidity(t *testing.T) {
	t.Run("various expiration times", func(t *testing.T) {
		tests := []struct {
			name        string
			notBefore   time.Time
			notAfter    time.Time
			expectValid bool
		}{
			{
				name:        "far future expiration",
				notBefore:   time.Now().Add(-time.Hour),
				notAfter:    time.Now().Add(365 * 24 * time.Hour), // 1 year
				expectValid: true,
			},
			{
				name:        "near expiration",
				notBefore:   time.Now().Add(-2 * time.Hour),
				notAfter:    time.Now().Add(10 * time.Minute), // 10 minutes left (should trigger renewal)
				expectValid: false,
			},
			{
				name:        "already expired",
				notBefore:   time.Now().Add(-2 * time.Hour),
				notAfter:    time.Now().Add(-time.Hour), // Expired
				expectValid: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				env, cleanup := setupTestConfig(t)
				defer cleanup()

				// Generate certificate with specific times
				certPEM, keyPEM := generateTestCertificate(t, tt.notBefore, tt.notAfter)

				// Save and load certificate
				err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
				require.NoError(t, err)

				err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
				require.NoError(t, err)

				// Check validity
				valid, nextCheck, err := env.cfg.CheckCertificateValidity(env.ctx)
				require.NoError(t, err)

				assert.Equal(t, tt.expectValid, valid, "validity should match expected")
				assert.Greater(t, nextCheck, time.Duration(0), "nextCheck should be positive")
			})
		}
	})

	t.Run("renewal timing calculation", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Generate certificate valid for 90 days
		notBefore := time.Now().Add(-time.Hour)
		notAfter := time.Now().Add(90 * 24 * time.Hour)
		certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter)

		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
		require.NoError(t, err)

		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		require.NoError(t, err)

		// Check certificate dates
		notBeforeActual, notAfterActual, remaining, err := env.cfg.CertificateDates()
		require.NoError(t, err)

		// Verify times are approximately correct (within 1 minute tolerance)
		assert.WithinDuration(t, notBefore, notBeforeActual, time.Minute)
		assert.WithinDuration(t, notAfter, notAfterActual, time.Minute)
		assert.Greater(t, remaining, 89*24*time.Hour) // Should be close to 90 days

		// Check validity calculation
		valid, nextCheck, err := env.cfg.CheckCertificateValidity(env.ctx)
		require.NoError(t, err)

		// Certificate should be valid (more than 1/3 of lifetime remaining)
		assert.True(t, valid)
		assert.Greater(t, nextCheck, time.Hour) // Should check again later
	})

	t.Run("clock skew handling", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Generate certificate that starts slightly in the future
		notBefore := time.Now().Add(5 * time.Minute) // 5 minutes in future
		notAfter := time.Now().Add(24 * time.Hour)
		certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter)

		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
		require.NoError(t, err)

		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		require.NoError(t, err)

		// Should handle future start time gracefully
		valid, _, err := env.cfg.CheckCertificateValidity(env.ctx)
		require.NoError(t, err)

		// Certificate should still be considered valid
		assert.True(t, valid, "certificate should be valid despite future notBefore")
	})

	t.Run("invalid certificate rejection", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Generate certificate that's already expired
		notBefore := time.Now().Add(-48 * time.Hour) // 2 days ago
		notAfter := time.Now().Add(-24 * time.Hour)  // 1 day ago
		certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter)

		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
		require.NoError(t, err)

		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		require.NoError(t, err)

		// Should detect expired certificate
		valid, _, err := env.cfg.CheckCertificateValidity(env.ctx)
		require.NoError(t, err)
		assert.False(t, valid, "expired certificate should be invalid")
	})
}

func TestCertificateRenewalFlow(t *testing.T) {
	t.Run("automatic renewal triggering", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Generate certificate that needs renewal (short validity)
		// Certificate lifetime = 3 hours, renewal at 1/3 = 1 hour before expiration
		// So with 30 minutes left, it should need renewal
		notBefore := time.Now().Add(-3 * time.Hour)
		notAfter := time.Now().Add(30 * time.Minute) // 30 minutes left (should trigger renewal)
		certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter)

		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
		require.NoError(t, err)

		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		require.NoError(t, err)

		// Check that renewal is needed
		valid, nextCheck, err := env.cfg.CheckCertificateValidity(env.ctx)
		require.NoError(t, err)

		assert.False(t, valid, "certificate should need renewal")
		assert.Greater(t, nextCheck, time.Duration(0), "should have next check time")
		assert.Less(t, nextCheck, 3*time.Hour, "next check should be soon")
	})

	t.Run("certificate replacement atomicity", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Save initial certificate
		notBefore1 := time.Now().Add(-time.Hour)
		notAfter1 := time.Now().Add(24 * time.Hour)
		certPEM1, keyPEM1 := generateTestCertificate(t, notBefore1, notAfter1)

		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM1, keyPEM1)
		require.NoError(t, err)

		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		require.NoError(t, err)

		// Get initial certificate dates
		_, notAfter1Loaded, _, err := env.cfg.CertificateDates()
		require.NoError(t, err)

		// Replace with new certificate
		notBefore2 := time.Now().Add(-30 * time.Minute)
		notAfter2 := time.Now().Add(48 * time.Hour) // Different expiration
		certPEM2, keyPEM2 := generateTestCertificate(t, notBefore2, notAfter2)

		err = env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM2, keyPEM2)
		require.NoError(t, err)

		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		require.NoError(t, err)

		// Verify new certificate is loaded
		_, notAfter2Loaded, _, err := env.cfg.CertificateDates()
		require.NoError(t, err)

		// Should have different expiration times
		assert.NotEqual(t, notAfter1Loaded, notAfter2Loaded, "certificate should be replaced")
		assert.WithinDuration(t, notAfter2, notAfter2Loaded, time.Minute)
	})

	t.Run("notification on certificate status change", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Initially no certificate
		assert.False(t, env.cfg.HaveCertificate(), "should start with no certificate")

		// Set up waiter for certificate change
		waiter := env.cfg.WaitForConfigChange(env.ctx)
		defer waiter.Cancel()

		// Save and load certificate (this changes certificate status from false to true)
		notBefore := time.Now().Add(-time.Hour)
		notAfter := time.Now().Add(24 * time.Hour)
		certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter)

		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
		require.NoError(t, err)

		// Trigger reload to detect certificate addition
		ac := env.cfg.(*appConfig)
		err = ac.load(env.ctx)
		require.NoError(t, err)

		// Should get notification for certificate status change (none -> have certificate)
		assert.True(t, waitForEvent(t, waiter, 1*time.Second),
			"Should receive notification when certificate status changes")

		// Verify certificate is now available
		assert.True(t, env.cfg.HaveCertificate(), "should have certificate after loading")
	})
}

func TestTLSConfiguration(t *testing.T) {
	t.Run("GetClientCertificate callback", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Generate test certificate
		notBefore := time.Now().Add(-time.Hour)
		notAfter := time.Now().Add(24 * time.Hour)
		certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter)

		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
		require.NoError(t, err)

		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		require.NoError(t, err)

		// Test GetClientCertificate callback
		certInfo := &tls.CertificateRequestInfo{
			AcceptableCAs: [][]byte{},
		}

		cert, err := env.cfg.GetClientCertificate(certInfo)
		require.NoError(t, err)
		assert.NotNil(t, cert, "should return certificate")
		assert.NotEmpty(t, cert.Certificate, "certificate should have data")
	})

	t.Run("GetCertificate for server mode", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Generate test certificate
		notBefore := time.Now().Add(-time.Hour)
		notAfter := time.Now().Add(24 * time.Hour)
		certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter)

		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
		require.NoError(t, err)

		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		require.NoError(t, err)

		// Test GetCertificate callback
		hello := &tls.ClientHelloInfo{
			ServerName: "test.example.com",
		}

		cert, err := env.cfg.GetCertificate(hello)
		require.NoError(t, err)
		assert.NotNil(t, cert, "should return certificate")
		assert.NotEmpty(t, cert.Certificate, "certificate should have data")
	})

	t.Run("certificate chain validation", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Generate test certificate
		notBefore := time.Now().Add(-time.Hour)
		notAfter := time.Now().Add(24 * time.Hour)
		certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter)

		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
		require.NoError(t, err)

		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		require.NoError(t, err)

		// Verify certificate can be used in TLS config
		certInfo := &tls.CertificateRequestInfo{}
		cert, err := env.cfg.GetClientCertificate(certInfo)
		require.NoError(t, err)
		require.NotNil(t, cert, "should return certificate")

		// Create TLS config with certificate
		tlsConfig := &tls.Config{
			GetClientCertificate: env.cfg.GetClientCertificate,
			GetCertificate:       env.cfg.GetCertificate,
		}

		assert.NotNil(t, tlsConfig.GetClientCertificate)
		assert.NotNil(t, tlsConfig.GetCertificate)
	})

	t.Run("no certificate available", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Don't load any certificate

		// GetClientCertificate should handle missing certificate gracefully
		certInfo := &tls.CertificateRequestInfo{}
		cert, err := env.cfg.GetClientCertificate(certInfo)

		// Should return error when no certificate is available
		assert.Error(t, err)
		assert.Nil(t, cert)

		// GetCertificate should also handle missing certificate
		hello := &tls.ClientHelloInfo{}
		cert, err = env.cfg.GetCertificate(hello)

		assert.Error(t, err)
		assert.Nil(t, cert)
	})
}

func TestCertificateEdgeCases(t *testing.T) {
	t.Run("zero-length certificate files", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Create empty certificate files
		stateDir := filepath.Join(env.tmpDir, depenv.DeployDevel.String())
		err := os.MkdirAll(stateDir, 0o700)
		require.NoError(t, err)

		certPath := filepath.Join(stateDir, "cert.pem")
		keyPath := filepath.Join(stateDir, "key.pem")

		err = os.WriteFile(certPath, []byte{}, 0o600)
		require.NoError(t, err)

		err = os.WriteFile(keyPath, []byte{}, 0o600)
		require.NoError(t, err)

		// Loading should fail gracefully
		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		assert.Error(t, err, "should fail with empty certificate files")
	})

	t.Run("certificate file permissions", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Generate test certificate
		notBefore := time.Now().Add(-time.Hour)
		notAfter := time.Now().Add(24 * time.Hour)
		certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter)

		// Save certificates
		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
		require.NoError(t, err)

		// Check file permissions
		keyPath := filepath.Join(env.tmpDir, depenv.DeployDevel.String(), "key.pem")
		info, err := os.Stat(keyPath)
		require.NoError(t, err)

		// Key file should have restrictive permissions (0600)
		// Note: On some systems, umask might affect the actual permissions
		perm := info.Mode().Perm()
		assert.True(t, perm&0o077 == 0, "private key should not be readable by group/other, got %o", perm)
	})

	t.Run("certificate dates edge cases", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Test with no certificate loaded
		_, _, _, err := env.cfg.CertificateDates()
		assert.ErrorIs(t, err, apitls.ErrNoCertificate, "should return ErrNoCertificate")

		// Test CheckCertificateValidity with no certificate
		valid, _, err := env.cfg.CheckCertificateValidity(env.ctx)
		assert.ErrorIs(t, err, apitls.ErrNoCertificate, "should return ErrNoCertificate")
		assert.False(t, valid)
	})

	t.Run("HaveCertificate state management", func(t *testing.T) {
		env, cleanup := setupTestConfig(t)
		defer cleanup()

		// Initially should not have certificate
		assert.False(t, env.cfg.HaveCertificate())

		// Generate and load certificate
		notBefore := time.Now().Add(-time.Hour)
		notAfter := time.Now().Add(24 * time.Hour)
		certPEM, keyPEM := generateTestCertificate(t, notBefore, notAfter)

		err := env.cfg.(*appConfig).SaveCertificates(env.ctx, certPEM, keyPEM)
		require.NoError(t, err)

		// Still shouldn't have certificate until loaded
		assert.False(t, env.cfg.HaveCertificate())

		err = env.cfg.(*appConfig).LoadCertificates(env.ctx)
		require.NoError(t, err)

		// Now should have certificate
		assert.True(t, env.cfg.HaveCertificate())
	})
}
