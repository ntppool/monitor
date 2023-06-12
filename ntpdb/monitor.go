package ntpdb

import (
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch"
	"inet.af/netaddr"

	"go.ntppool.org/monitor/api/pb"
)

type MonitorConfig struct {
	Samples    int32    `json:"samples"`
	NatIP      string   `json:"nat_ip,omitempty"` // have the monitor bind to a different IP
	BaseChecks []string `json:"base_checks,omitempty"`
	ip         string
	MQTT       *pb.MQTTConfig
}

func (m *Monitor) IsLive() bool {
	switch m.Status {
	case MonitorsStatusActive, MonitorsStatusTesting:
		return true
	default:
		return false
	}
}

func (m *Monitor) GetConfigWithDefaults(defaults []byte) (*MonitorConfig, error) {

	merged, err := jsonpatch.MergePatch(defaults, []byte(m.Config))
	if err != nil {
		return nil, err
	}

	// fmt.Printf("mx: %s\n", merged)

	return m.getConfig(merged)

}

func (m *Monitor) GetConfig() (*MonitorConfig, error) {
	return m.getConfig([]byte(m.Config))
}

func (m *Monitor) getConfig(conf []byte) (*MonitorConfig, error) {
	moncfg := &MonitorConfig{}
	err := json.Unmarshal([]byte(conf), moncfg)
	if err != nil {
		return nil, err
	}

	moncfg.ip = m.Ip.String

	return moncfg, nil
}

func (cfg *MonitorConfig) PbConfig() (*pb.Config, error) {

	rcfg := &pb.Config{
		Samples: cfg.Samples,
	}

	if len(cfg.BaseChecks) > 0 {
		for _, s := range cfg.BaseChecks {
			rcfg.BaseChecks = append(rcfg.BaseChecks, []byte(s))
		}
	}

	if len(cfg.NatIP) > 0 {
		ip, err := netaddr.ParseIP(cfg.NatIP)
		if err != nil {
			return nil, err
		}
		rcfg.IPNatBytes, _ = ip.MarshalBinary()
	}

	ip, err := netaddr.ParseIP(cfg.ip)
	if err != nil {
		return nil, err
	}
	rcfg.IPBytes, _ = ip.MarshalBinary()

	if cfg.MQTT != nil {
		rcfg.MQTTConfig = cfg.MQTT
	}

	return rcfg, nil
}
