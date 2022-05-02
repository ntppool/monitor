package sctx

type contextKey int

const (
	CertificateKey contextKey = iota
	MonitorKey
	ClientVersion
)
