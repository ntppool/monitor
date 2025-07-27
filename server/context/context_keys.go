package sctx

type contextKey int

const (
	CertificateKey contextKey = iota
	MonitorKey
	ClientVersionKey
	RequestStartKey
	DeploymentEnv
	AccountKey
	AuthMethodKey
	JWTClaimsKey
)
