package api

var apiServers = map[DeploymentEnvironment]string{
	DeployDevel: "https://api.devel.mon.ntppool.dev",
	DeployTest:  "https://api.test.mon.ntppool.dev",
	DeployProd:  "https://api.mon.ntppool.dev",
}

const (
	DeployUndefined DeploymentEnvironment = iota
	DeployDevel
	DeployTest
	DeployProd
)

type DeploymentEnvironment uint8

func DeploymentEnvironmentFromString(s string) (DeploymentEnvironment, error) {
	switch s {
	case "devel":
		return DeployDevel, nil
	case "test":
		return DeployTest, nil
	case "prod":
		return DeployProd, nil
	default:
		return DeployUndefined, nil
	}
}

func (d DeploymentEnvironment) String() string {
	switch d {
	case DeployProd:
		return "prod"
	case DeployTest:
		return "test"
	case DeployDevel:
		return "devel"
	default:
		panic("invalid DeploymentEnvironment")
	}
}
