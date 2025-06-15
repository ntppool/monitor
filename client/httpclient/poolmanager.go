package httpclient

import (
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"syscall"

	"go.ntppool.org/common/logger"
)

// PoolFlusherTransport wraps an http.Transport and provides the ability to flush connection pools
// when certificate or connection errors are detected.
type PoolFlusherTransport struct {
	*http.Transport
	log *slog.Logger
}

// NewPoolFlusherTransport creates a new transport wrapper that can flush connection pools
func NewPoolFlusherTransport(transport *http.Transport) *PoolFlusherTransport {
	return &PoolFlusherTransport{
		Transport: transport,
		log:       logger.Setup().WithGroup("pool-flusher"),
	}
}

// RoundTrip implements http.RoundTripper and detects errors that should trigger connection pool flushing
func (pft *PoolFlusherTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := pft.Transport.RoundTrip(req)

	if shouldFlushConnections(resp, err) {
		ctx := req.Context()
		pft.log.InfoContext(ctx, "detected certificate/connection error, flushing connection pool",
			"url", req.URL.String(),
			"error", err,
			"status", getStatusCode(resp))

		// Close idle connections to force fresh connections on retry
		pft.Transport.CloseIdleConnections()

		// Retry the request with fresh connections
		if err != nil {
			pft.log.DebugContext(ctx, "retrying request after pool flush")
			resp, err = pft.Transport.RoundTrip(req)
		}
	}

	return resp, err
}

// shouldFlushConnections determines if we should flush the connection pool based on the response or error
func shouldFlushConnections(resp *http.Response, err error) bool {
	// Check for TLS-related errors
	if err != nil {
		// TLS handshake failures
		if isTLSError(err) {
			return true
		}

		// Connection refused/reset errors that might indicate certificate issues
		if isConnectionError(err) {
			return true
		}
	}

	// Check for HTTP status codes that indicate certificate/auth issues
	if resp != nil {
		switch resp.StatusCode {
		case 495: // SSL Certificate Error (nginx)
			return true
		case 496: // SSL Certificate Required (nginx)
			return true
		case 497: // HTTP Request Sent to HTTPS Port (nginx)
			return true
		case 401: // Unauthorized - could be cert auth failure
			return true
		case 403: // Forbidden - could be cert auth failure
			return true
		}
	}

	return false
}

// isTLSError checks if the error is related to TLS/certificate issues
func isTLSError(err error) bool {
	if err == nil {
		return false
	}

	// Check for tls.RecordHeaderError, certificate verification errors, etc.
	var tlsErr *tls.RecordHeaderError
	if errors.As(err, &tlsErr) {
		return true
	}

	// Check for certificate verification errors
	if strings.Contains(err.Error(), "certificate") ||
		strings.Contains(err.Error(), "tls:") ||
		strings.Contains(err.Error(), "x509:") {
		return true
	}

	return false
}

// isConnectionError checks if the error indicates a connection-level problem
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}

	// Check for connection refused, reset, etc.
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "connection reset") {
		return true
	}

	return false
}

// getStatusCode safely extracts status code from response for logging
func getStatusCode(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}
