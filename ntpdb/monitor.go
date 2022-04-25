package ntpdb

import (
	"encoding/json"

	"go.ntppool.org/monitor/api/pb"
	"inet.af/netaddr"
)

type MonitorConfig struct {
	Samples int32 `json:"samples"`
}

func (m *Monitor) IsLive() bool {
	switch m.Status {
	case MonitorsStatusActive, MonitorsStatusTesting:
		return true
	default:
		return false
	}
}

func (m *Monitor) GetConfig() (*MonitorConfig, error) {

	moncfg := &MonitorConfig{}

	err := json.Unmarshal([]byte(m.Config), moncfg)
	if err != nil {
		return nil, err
	}

	return moncfg, nil
}

func (m *Monitor) GetPbConfig() (*pb.Config, error) {

	cfg, err := m.GetConfig()
	if err != nil {
		return nil, err
	}

	rcfg := &pb.Config{
		Samples: cfg.Samples,
	}

	ip, err := netaddr.ParseIP(m.Ip)
	if err != nil {
		return nil, err
	}
	rcfg.IPBytes, _ = ip.MarshalBinary()
	return rcfg, nil
}
