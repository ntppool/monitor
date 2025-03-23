// This package provides an HTTP client that can be configured
// to use a specific IP version (IPv4 or IPv6) when making
// requests.
package httpclient

import (
	"context"
	"net"
	"net/http"
	"time"
)

// IPVersion is used to specify which IP version to use
type IPVersion int

const (
	// IPAny allows connections over either IPv4 or IPv6
	IPAny IPVersion = iota
	// IPv4Only forces connections over IPv4 only
	IPv4Only
	// IPv6Only forces connections over IPv6 only
	IPv6Only
)

// ipVersionContextKey is a context key used to pass IP version preference
type ipVersionContextKey struct{}

// NewIPVersionContext creates a new context with IP version preference
func NewIPVersionContext(ctx context.Context, version IPVersion) context.Context {
	return context.WithValue(ctx, ipVersionContextKey{}, version)
}

// getIPVersionFromContext extracts the IP version from context
func getIPVersionFromContext(ctx context.Context) IPVersion {
	if value := ctx.Value(ipVersionContextKey{}); value != nil {
		if version, ok := value.(IPVersion); ok {
			return version
		}
	}
	return IPAny // Default to any IP version
}

// CreateIPVersionAwareClient creates an HTTP client that respects IP version preference
func CreateIPVersionAwareClient() *http.Client {
	transport := &http.Transport{
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		IdleConnTimeout:       120 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 40 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Override the default dial function to check context for IP version preference
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		ipVersion := getIPVersionFromContext(ctx)

		// Determine which network type to use based on context value
		switch ipVersion {
		case IPv4Only:
			network = "tcp4"
		case IPv6Only:
			network = "tcp6"
			// Otherwise, use whatever was provided (probably "tcp" which allows both)
		}

		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}

		conn, err := dialer.DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		return conn, nil
	}

	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}
}
