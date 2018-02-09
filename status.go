package monitor

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type ServerList struct {
	Config  *Config  `json:"config"`
	Servers []string `json:"servers"`
}

type ServerStatus struct {
	TS         time.Time     `json:"ts"`
	Server     *net.IP       `json:"server"`
	Offset     time.Duration `json:"offset"`
	RTT        time.Duration `json:"rtt,omitempty"`
	Stratum    uint8         `json:"stratum"`
	Leap       uint8         `json:"leap"`
	Error      string        `json:"error,omitempty"`
	NoResponse bool          `json:"no_response"`
}

type MonitorFeedback struct {
	Version int             `json:"version"`
	Servers []*ServerStatus `json:"servers"`
}

func (s *ServerStatus) MarshalJSON() ([]byte, error) {
	if s.TS.IsZero() {
		return nil, fmt.Errorf("TS is zero")
	}
	return json.Marshal(&struct {
		TS         int64   `json:"ts"`
		Server     string  `json:"server"`
		Offset     float64 `json:"offset"`
		RTT        float64 `json:"rtt,omitempty"`
		Stratum    uint8   `json:"stratum"`
		Leap       uint8   `json:"leap"`
		Error      string  `json:"error,omitempty"`
		NoResponse bool    `json:"no_response"`
	}{
		TS:         s.TS.Unix(),
		Server:     s.Server.String(),
		Offset:     float64(s.Offset) / float64(time.Second),
		RTT:        float64(s.RTT) / float64(time.Second),
		Stratum:    s.Stratum,
		Leap:       s.Leap,
		Error:      s.Error,
		NoResponse: s.NoResponse,
	})
}

func (sl *ServerList) IPs() ([]*net.IP, error) {
	servers := []*net.IP{}

	for _, s := range sl.Servers {
		sip := net.ParseIP(s)
		if sip == nil {
			return nil, fmt.Errorf("invalid IP %q", s)
		}
		servers = append(servers, &sip)
	}
	return servers, nil
}
