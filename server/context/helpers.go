package sctx

import (
	"context"
	"log"

	"go.ntppool.org/monitor/api"
)

func GetDeploymentEnvironment(ctx context.Context) api.DeploymentEnvironment {
	depEnv := ctx.Value(DeploymentEnv)
	if e, ok := depEnv.(api.DeploymentEnvironment); ok {
		return e
	}
	log.Fatalf("DeploymentEnv unavailable in context")
	return api.DeployUndefined
}
