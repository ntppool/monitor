package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v5"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
)

// OIDCTokenRequest represents the request body for the OIDC token endpoint
type OIDCTokenRequest struct {
	ExpiresIn int `json:"expires_in"`
}

// OIDCTokenResponse represents the response from the OIDC token endpoint
type OIDCTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// JWT token refresh buffer - refresh 5 minutes before expiration
const jwtRefreshBuffer = 5 * time.Minute

// Default JWT token duration - 1 hour
const defaultJWTExpiresIn = 3600

// GetJWTToken returns a valid JWT token, refreshing if necessary
func (ac *appConfig) GetJWTToken(ctx context.Context) (string, error) {
	ctx, span := tracing.Start(ctx, "GetJWTToken")
	defer span.End()

	// Check if we have a valid token
	ac.jwtMutex.RLock()
	if ac.jwtToken != "" && time.Until(ac.jwtExpiry) > jwtRefreshBuffer {
		token := ac.jwtToken
		ac.jwtMutex.RUnlock()
		return token, nil
	}
	ac.jwtMutex.RUnlock()

	// Need to refresh the token
	return ac.refreshJWTToken(ctx)
}

// refreshJWTToken fetches a new JWT token from the OIDC endpoint
func (ac *appConfig) refreshJWTToken(ctx context.Context) (string, error) {
	ctx, span := tracing.Start(ctx, "refreshJWTToken")
	defer span.End()
	log := logger.FromContext(ctx)

	// Only one goroutine should refresh at a time
	ac.jwtMutex.Lock()
	defer ac.jwtMutex.Unlock()

	// Check again if someone else already refreshed while we were waiting
	if ac.jwtToken != "" && time.Until(ac.jwtExpiry) > jwtRefreshBuffer {
		return ac.jwtToken, nil
	}

	// Get API key for authentication
	apiKey := ac.APIKey()
	if apiKey == "" {
		return "", errors.New("no API key available for JWT token request")
	}

	// Build OIDC token endpoint URL
	baseURL := ac.e.APIHost()
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	tokenURL := baseURL + "api/oidc/token"

	// Prepare request body
	reqBody := OIDCTokenRequest{
		ExpiresIn: defaultJWTExpiresIn,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OIDC token request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create OIDC token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	log.DebugContext(ctx, "requesting JWT token", "endpoint", tokenURL)

	// Make the request
	resp, err := apiHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("OIDC token request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WarnContext(ctx, "failed to close OIDC response body", "err", err)
		}
	}()

	traceID := resp.Header.Get("Traceid")

	// Handle different response codes
	switch resp.StatusCode {
	case http.StatusOK:
		// Success - parse response
	case http.StatusUnauthorized:
		log.WarnContext(ctx, "JWT token request unauthorized", "trace", traceID)
		return "", ErrAuthorization
	default:
		return "", fmt.Errorf("unexpected OIDC token response: %d (trace %s)", resp.StatusCode, traceID)
	}

	// Parse response
	var tokenResp OIDCTokenResponse
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode OIDC token response: %w", err)
	}

	// Validate response
	if tokenResp.AccessToken == "" {
		return "", errors.New("OIDC response missing access_token")
	}
	if tokenResp.TokenType != "Bearer" {
		log.WarnContext(ctx, "unexpected token type", "type", tokenResp.TokenType)
	}

	// Calculate expiry time
	expiresIn := time.Duration(tokenResp.ExpiresIn) * time.Second
	if expiresIn == 0 {
		expiresIn = time.Duration(defaultJWTExpiresIn) * time.Second
	}
	expiryTime := time.Now().Add(expiresIn)

	// Store the new token
	ac.jwtToken = tokenResp.AccessToken
	ac.jwtExpiry = expiryTime

	log.DebugContext(ctx, "JWT token refreshed successfully",
		"expires_in", expiresIn.Round(time.Minute).String(),
		"expires_at", expiryTime.Format("15:04:05"))

	return ac.jwtToken, nil
}

// clearJWTToken clears the stored JWT token (used on authorization errors)
func (ac *appConfig) clearJWTToken() {
	ac.jwtMutex.Lock()
	defer ac.jwtMutex.Unlock()

	ac.jwtToken = ""
	ac.jwtExpiry = time.Time{}
}

// jwtTokenNeedsRefresh checks if the JWT token needs to be refreshed
func (ac *appConfig) jwtTokenNeedsRefresh() bool {
	ac.jwtMutex.RLock()
	defer ac.jwtMutex.RUnlock()

	if ac.jwtToken == "" {
		return true
	}

	return time.Until(ac.jwtExpiry) <= jwtRefreshBuffer
}

// waitForJWTToken waits for a valid JWT token to be available, similar to waitUntilAPIKey
func (ac *appConfig) waitForJWTToken(ctx context.Context) error {
	ctx, span := tracing.Start(ctx, "waitForJWTToken")
	defer span.End()
	log := logger.FromContext(ctx)

	// Check if we already have a valid token
	if !ac.jwtTokenNeedsRefresh() {
		return nil
	}

	// Backoff for JWT token refresh failures
	jwtBackoff := newConfigBackoff(2*time.Second, 2*time.Minute)

	log.DebugContext(ctx, "waiting for JWT token to be available")

	for {
		// Try to refresh the token
		_, err := ac.refreshJWTToken(ctx)
		if err != nil {
			if errors.Is(err, ErrAuthorization) {
				// Authorization error - API key might be invalid
				log.WarnContext(ctx, "JWT token request authorization failed", "err", err)
				return err
			}
			// Other errors - continue waiting with backoff
			log.DebugContext(ctx, "JWT token refresh failed, will retry", "err", err)
		} else {
			// Success
			log.DebugContext(ctx, "JWT token is now available")
			return nil
		}

		// Wait before retrying
		waitTime := jwtBackoff.NextBackOff()
		if waitTime == backoff.Stop {
			waitTime = jwtBackoff.MaxInterval
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Continue to next iteration
		}
	}
}
