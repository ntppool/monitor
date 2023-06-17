package config

import (
	"sync"
	"time"

	"go.ntppool.org/monitor/api/pb"
)

type MQConfigger interface {
	GetMQTTConfig() *pb.MQTTConfig
}

type ConfigUpdater interface {
	SetConfig(cfg *pb.Config)
	GetConfig() *pb.Config
}

type Configger struct {
	cfg     *pb.Config
	lastSet time.Time
	sync.RWMutex
}

func NewConfigger(cfg *pb.Config) *Configger {
	return &Configger{cfg: cfg}
}

func (c *Configger) GetConfig() *pb.Config {
	c.RLock()
	defer c.RUnlock()
	return c.cfg
}

func (c *Configger) GetMQTTConfig() *pb.MQTTConfig {
	cfg := c.GetConfig()
	if cfg == nil {
		return nil
	}
	return cfg.MQTTConfig
}

func (c *Configger) SetConfig(cfg *pb.Config) {
	c.Lock()
	defer c.Unlock()

	c.lastSet = time.Now()

	var mqtt *pb.MQTTConfig
	if c.cfg != nil && c.cfg.MQTTConfig != nil {
		mqtt = c.cfg.MQTTConfig
	}

	c.cfg = cfg
	if c.cfg.MQTTConfig == nil && mqtt != nil {
		c.cfg.MQTTConfig = mqtt
	}
}
