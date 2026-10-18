package sctx

type contextKey int

const (
	CertificateKey contextKey = iota
	ClientVersionKey
	DeploymentEnv
	AuthMethodKey
	JWTClaimsKey
)
