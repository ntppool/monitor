package sctx

import (
	"context"

	"go.ntppool.org/common/config/depenv"
	"go.ntppool.org/common/logger"
)

func GetDeploymentEnvironment(ctx context.Context) depenv.DeploymentEnvironment {
	depEnv := ctx.Value(DeploymentEnv)
	if e, ok := depEnv.(depenv.DeploymentEnvironment); ok {
		return e
	}
	logger.Setup().Error("DeploymentEnv unavailable in context")
	return depenv.DeployUndefined
}
