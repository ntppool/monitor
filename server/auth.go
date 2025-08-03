package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
	sctx "go.ntppool.org/monitor/server/context"
)

const (
	// Maximum reasonable JWT token size (3KB)
	maxJWTTokenSize = 3072
)

// AuthMethod represents the authentication method used
type AuthMethod string

const (
	AuthMethodMTLS AuthMethod = "mtls"
	AuthMethodJWT  AuthMethod = "jwt"
)

// JWTClaims represents the expected JWT claims structure
type JWTClaims struct {
	jwt.RegisteredClaims
	Monitor string `json:"monitor,omitempty"`
	Scope   string `json:"scope,omitempty"`
}

// JWTAuthenticator handles JWT token validation using JWKS
type JWTAuthenticator struct {
	jwks   keyfunc.Keyfunc
	issuer string
}

// NewJWTAuthenticator creates a new JWT authenticator with JWKS support
func NewJWTAuthenticator(ctx context.Context, deploymentEnv depenv.DeploymentEnvironment) (*JWTAuthenticator, error) {
	log := logger.FromContext(ctx)

	if deploymentEnv == depenv.DeployUndefined {
		return nil, fmt.Errorf("invalid deployment environment: %s", deploymentEnv)
	}

	issuer := deploymentEnv.APIHost()
	jwksURL := issuer + "/.well-known/jwks.json"

	log.DebugContext(ctx, "initializing JWT authenticator", "issuer", issuer, "jwks_url", jwksURL)

	// Create JWKS client with automatic refresh
	jwks, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("failed to create JWKS client: %w", err)
	}

	return &JWTAuthenticator{
		jwks:   jwks,
		issuer: issuer,
	}, nil
}

// ValidateToken validates a JWT token and returns the claims
func (j *JWTAuthenticator) ValidateToken(ctx context.Context, tokenString string) (*JWTClaims, error) {
	log := logger.FromContext(ctx)

	// Basic size validation before expensive parsing
	if len(tokenString) > maxJWTTokenSize {
		return nil, fmt.Errorf("token too large: %d bytes (max %d)", len(tokenString), maxJWTTokenSize)
	}
	if len(tokenString) == 0 {
		return nil, errors.New("token is empty")
	}

	// Parse and validate the token
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, j.jwks.Keyfunc, jwt.WithValidMethods([]string{"ES384", "RS256"}))
	if err != nil {
		log.DebugContext(ctx, "JWT token parsing failed", "error", err.Error())
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Extract claims
	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	// Validate issuer
	if claims.Issuer != j.issuer {
		log.WarnContext(ctx, "JWT token has invalid issuer", "expected", j.issuer, "actual", claims.Issuer)
		return nil, fmt.Errorf("invalid issuer: expected %s, got %s", j.issuer, claims.Issuer)
	}

	// Validate audience
	validAudience := false
	for _, aud := range claims.Audience {
		if aud == "monitor-api" {
			validAudience = true
			break
		}
	}
	if !validAudience {
		log.WarnContext(ctx, "JWT token missing required audience", "audiences", claims.Audience)
		return nil, errors.New("token missing required audience 'monitor-api'")
	}

	// Validate scope - must contain at least one valid monitor scope
	hasValidScope := strings.Contains(claims.Scope, "monitor:read") || strings.Contains(claims.Scope, "monitor:data")
	if !hasValidScope {
		log.WarnContext(ctx, "JWT token missing valid monitor scope", "scope", claims.Scope)
		return nil, errors.New("token missing valid monitor scope")
	}

	// Validate claims format
	if err := validateJWTClaims(claims); err != nil {
		log.WarnContext(ctx, "JWT token claims validation failed", "error", err.Error())
		return nil, fmt.Errorf("invalid claims: %w", err)
	}

	log.DebugContext(ctx, "JWT token validation successful",
		"subject", claims.Subject,
		"monitor", claims.Monitor,
		"scope", claims.Scope)

	return claims, nil
}

// Close closes the JWKS client
func (j *JWTAuthenticator) Close() {
	// keyfunc v3 handles cleanup automatically
}

// validateJWTClaims performs basic validation on JWT claims
func validateJWTClaims(claims *JWTClaims) error {
	// Validate Subject is not empty (required for identification)
	if claims.Subject == "" {
		return errors.New("subject claim is required")
	}

	if len(claims.Subject) > 128 {
		return errors.New("subject too long (max 128 characters)")
	}

	// Validate Monitor field length if present
	if len(claims.Monitor) > 64 {
		return errors.New("monitor name too long (max 64 characters)")
	}

	return nil
}

// dualAuthMiddleware provides both JWT and mTLS authentication support
// mTLS takes precedence over JWT when both are present
func (srv *Server) dualAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		// SECURITY ASSUMPTION: If we reach this HTTP middleware AND the client
		// sent certificates in r.TLS.PeerCertificates, it means:
		// 1. The TLS handshake completed successfully
		// 2. Our verifyClient callback was called and returned nil (success)
		// 3. The certificates were validated against our CA pool
		// 4. The certificates meet our DNS suffix requirements (.mon.ntppool.dev)
		//
		// This is because with RequestClientCert + VerifyPeerCertificate callback:
		// - If verifyClient returns error → TLS handshake fails → no HTTP request
		// - If verifyClient returns nil → TLS handshake succeeds → HTTP request proceeds

		// Check for mTLS authentication first (takes precedence over JWT)
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
			// Client sent certificate(s) and TLS handshake succeeded
			// This proves our verifyClient callback validated them successfully

			// Extract certificate identity (no need to re-validate, already done in TLS layer)
			cert, name := srv.getVerifiedCertFromPeers(r.TLS.PeerCertificates)
			if cert != nil && name != "" {
				// Store mTLS authentication context
				ctx = context.WithValue(ctx, sctx.AuthMethodKey, AuthMethodMTLS)
				ctx = context.WithValue(ctx, sctx.CertificateKey, name)

				authHeader := r.Header.Get("Authorization")
				jwtHeaderPresent := strings.HasPrefix(authHeader, "Bearer ")

				log.DebugContext(ctx, "authentication successful",
					"method", "mtls",
					"certificate_name", name,
					"remote_addr", r.RemoteAddr,
					"user_agent", r.UserAgent(),
					"endpoint", r.URL.Path,
					"jwt_header_present", jwtHeaderPresent)

				r = r.WithContext(ctx)
				next.ServeHTTP(w, r)
				return
			}
			// Note: This case should be impossible given our security assumptions above
			log.WarnContext(ctx, "TLS handshake succeeded but certificate validation failed in middleware",
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent(),
				"endpoint", r.URL.Path)
		}

		// Fallback to JWT authentication if no client certificate was sent
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")

			if srv.jwtAuth != nil {
				claims, err := srv.jwtAuth.ValidateToken(ctx, tokenString)
				if err != nil {
					log.WarnContext(ctx, "authentication failed",
						"method", "jwt",
						"error", err.Error(),
						"remote_addr", r.RemoteAddr,
						"user_agent", r.UserAgent(),
						"endpoint", r.URL.Path)
					http.Error(w, "Invalid JWT token", http.StatusUnauthorized)
					return
				}

				// Store JWT authentication context
				ctx = context.WithValue(ctx, sctx.AuthMethodKey, AuthMethodJWT)
				ctx = context.WithValue(ctx, sctx.JWTClaimsKey, claims)
				ctx = context.WithValue(ctx, sctx.CertificateKey, claims.Subject)

				log.DebugContext(ctx, "authentication successful",
					"method", "jwt",
					"subject", claims.Subject,
					"monitor", claims.Monitor,
					"remote_addr", r.RemoteAddr,
					"user_agent", r.UserAgent(),
					"endpoint", r.URL.Path)

				r = r.WithContext(ctx)
				next.ServeHTTP(w, r)
				return
			}
		}

		// No valid authentication method found
		log.WarnContext(ctx, "authentication required",
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"endpoint", r.URL.Path)
		http.Error(w, "Authentication required", http.StatusUnauthorized)
	})
}

// getAuthMethod returns the authentication method used for the current request
func getAuthMethod(ctx context.Context) AuthMethod {
	if method, ok := ctx.Value(sctx.AuthMethodKey).(AuthMethod); ok {
		return method
	}
	return AuthMethodMTLS // Default fallback
}

// getJWTClaims returns the JWT claims if JWT authentication was used
func getJWTClaims(ctx context.Context) (*JWTClaims, bool) {
	claims, ok := ctx.Value(sctx.JWTClaimsKey).(*JWTClaims)
	return claims, ok
}
