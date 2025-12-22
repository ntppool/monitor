// checkconfig has the configuration data that's sent by the
// monitoring API. More of this (certainly the MQTT config)
// should be moved to the configuration API.
package checkconfig

import (
	"net/netip"
	"sync"
	"time"

	"go.ntppool.org/monitor/api/pb"
	apiv2 "go.ntppool.org/monitor/gen/monitor/v2"
)

type MQConfigger interface {
	GetMQTTConfig() *MQTTConfig
}

type MQTTConfig struct {
	Host   string
	Port   int
	JWT    []byte
	Prefix string
}
type Config struct {
	Samples    int32
	IP         *netip.Addr
	IPNat      *netip.Addr
	BaseChecks []string
	MQTTConfig *MQTTConfig
}

type ConfigGetter interface {
	GetConfig() *Config
	MQConfigger
}

type ConfigUpdater interface {
	SetConfigFromPb(cfg *pb.Config)
	SetConfigFromApi(cfg *apiv2.GetConfigResponse)
}
type ConfigProvider interface {
	ConfigUpdater
	ConfigGetter
}

type Configger struct {
	cfg          *Config
	lastSet      time.Time
	otlpLogLevel string
	sync.RWMutex
}

func NewConfigger(cfg *Config) *Configger {
	return &Configger{cfg: cfg}
}

func (c *Configger) GetConfig() *Config {
	c.RLock()
	defer c.RUnlock()
	return c.cfg
}

func (c *Configger) GetMQTTConfig() *MQTTConfig {
	cfg := c.GetConfig()
	if cfg == nil {
		return nil
	}
	return cfg.MQTTConfig
}

func (c *Configger) SetConfigFromApi(cfg *apiv2.GetConfigResponse) {
	c.Lock()
	defer c.Unlock()

	c.lastSet = time.Now()

	// keep mqttconfig as updating it is optional
	var mqtt *MQTTConfig
	if c.cfg != nil && c.cfg.MQTTConfig != nil {
		mqtt = c.cfg.MQTTConfig
	}

	c.cfg = &Config{
		Samples:    cfg.GetSamples(),
		IP:         cfg.GetIP(),
		IPNat:      cfg.GetNatIP(),
		MQTTConfig: mqtt,
	}

	for _, b := range cfg.BaseChecks {
		c.cfg.BaseChecks = append(c.cfg.BaseChecks, string(b))
	}

	if cfg.MqttConfig != nil {
		c.cfg.MQTTConfig = &MQTTConfig{
			Host:   string(cfg.MqttConfig.GetHost()),
			Port:   int(cfg.MqttConfig.GetPort()),
			JWT:    cfg.MqttConfig.GetJwt(),
			Prefix: string(cfg.MqttConfig.GetPrefix()),
		}
	}

	// restore saved config if necessary
	if c.cfg.MQTTConfig == nil && mqtt != nil {
		c.cfg.MQTTConfig = mqtt
	}

	// Store the OTLP log level (caller handles applying and persisting)
	c.otlpLogLevel = cfg.GetOtlpLogLevel()
}

// OtlpLogLevel returns the server-configured OTLP log level
func (c *Configger) OtlpLogLevel() string {
	c.RLock()
	defer c.RUnlock()
	return c.otlpLogLevel
}

func (c *Configger) SetConfigFromPb(cfg *pb.Config) {
	c.Lock()
	defer c.Unlock()

	c.lastSet = time.Now()

	// keep mqttconfig as updating it is optional
	var mqtt *MQTTConfig
	if c.cfg != nil && c.cfg.MQTTConfig != nil {
		mqtt = c.cfg.MQTTConfig
	}

	c.cfg = &Config{
		Samples:    cfg.GetSamples(),
		IP:         cfg.GetIP(),
		IPNat:      cfg.GetNatIP(),
		MQTTConfig: mqtt,
	}

	for _, b := range cfg.BaseChecks {
		c.cfg.BaseChecks = append(c.cfg.BaseChecks, string(b))
	}

	if cfg.MqttConfig != nil {
		c.cfg.MQTTConfig = &MQTTConfig{
			Host:   string(cfg.MqttConfig.GetHost()),
			Port:   int(cfg.MqttConfig.GetPort()),
			JWT:    cfg.MqttConfig.GetJwt(),
			Prefix: string(cfg.MqttConfig.GetPrefix()),
		}
	}

	// restore saved config if necessary
	if c.cfg.MQTTConfig == nil && mqtt != nil {
		c.cfg.MQTTConfig = mqtt
	}
}

func (mq *MQTTConfig) PbConfig() *pb.MQTTConfig {
	return &pb.MQTTConfig{
		Host:   []byte(mq.Host),
		Port:   int32(mq.Port),
		Jwt:    mq.JWT,
		Prefix: []byte(mq.Prefix),
	}
}

func (mq *MQTTConfig) ApiConfig() *apiv2.MQTTConfig {
	return &apiv2.MQTTConfig{
		Host:   []byte(mq.Host),
		Port:   int32(mq.Port),
		Jwt:    mq.JWT,
		Prefix: []byte(mq.Prefix),
	}
}
