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
func NewJWTAuthenticator(ctx context.Context, deploymentEnv string) (*JWTAuthenticator, error) {
	log := logger.FromContext(ctx)

	depEnv := depenv.DeploymentEnvironmentFromString(deploymentEnv)
	if depEnv == depenv.DeployUndefined {
		return nil, fmt.Errorf("invalid deployment environment: %s", deploymentEnv)
	}

	issuer := depEnv.APIHost()
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

	// Validate scope
	if !strings.Contains(claims.Scope, "monitor:read") {
		log.WarnContext(ctx, "JWT token missing required scope", "scope", claims.Scope)
		return nil, errors.New("token missing required scope 'monitor:read'")
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

// dualAuthMiddleware provides both JWT and mTLS authentication support
func (srv *Server) dualAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		// Check for JWT authentication first
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString := strings.TrimPrefix(authHeader, "Bearer ")

			if srv.jwtAuth != nil {
				claims, err := srv.jwtAuth.ValidateToken(ctx, tokenString)
				if err != nil {
					log.WarnContext(ctx, "JWT authentication failed", "error", err.Error())
					http.Error(w, "Invalid JWT token", http.StatusUnauthorized)
					return
				}

				// Store JWT authentication context
				ctx = context.WithValue(ctx, sctx.AuthMethodKey, AuthMethodJWT)
				ctx = context.WithValue(ctx, sctx.JWTClaimsKey, claims)
				ctx = context.WithValue(ctx, sctx.CertificateKey, claims.Subject)

				log.DebugContext(ctx, "JWT authentication successful", "subject", claims.Subject)

				r = r.WithContext(ctx)
				next.ServeHTTP(w, r)
				return
			}
		}

		// Fallback to mTLS authentication
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			log.WarnContext(ctx, "no valid authentication method found")
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// Verify mTLS certificate
		cert, name := srv.getVerifiedCert(r.TLS.VerifiedChains)
		if cert == nil || name == "" {
			log.WarnContext(ctx, "mTLS certificate verification failed")
			http.Error(w, "Invalid client certificate", http.StatusUnauthorized)
			return
		}

		// Store mTLS authentication context
		ctx = context.WithValue(ctx, sctx.AuthMethodKey, AuthMethodMTLS)
		ctx = context.WithValue(ctx, sctx.CertificateKey, name)

		log.DebugContext(ctx, "mTLS authentication successful", "certificate_name", name)

		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
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
