package ntpdb

import (
	"encoding/json"

	"go.ntppool.org/monitor/api/pb"
	"inet.af/netaddr"
)

type MonitorConfig struct {
	Samples int32  `json:"samples"`
	NatIP   string `json:"nat_ip,omitempty"` // have the monitor bind to a different IP
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

	if len(cfg.NatIP) > 0 {
		ip, err := netaddr.ParseIP(cfg.NatIP)
		if err != nil {
			return nil, err
		}
		rcfg.IPNatBytes, _ = ip.MarshalBinary()
	}

	ip, err := netaddr.ParseIP(m.Ip)
	if err != nil {
		return nil, err
	}
	rcfg.IPBytes, _ = ip.MarshalBinary()

	return rcfg, nil
}
