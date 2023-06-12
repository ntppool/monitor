package sctx

import (
	"context"

	"go.ntppool.org/monitor/api"
	"go.ntppool.org/monitor/logger"
)

func GetDeploymentEnvironment(ctx context.Context) api.DeploymentEnvironment {
	depEnv := ctx.Value(DeploymentEnv)
	if e, ok := depEnv.(api.DeploymentEnvironment); ok {
		return e
	}
	logger.Setup().Error("DeploymentEnv unavailable in context")
	return api.DeployUndefined
}
