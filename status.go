package monitor

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

type ServerStatus struct {
	TS         time.Time     `json:"ts"`
	Server     net.IP        `json:"server"`
	Offset     time.Duration `json:"offset"` // todo: convert to float64 of milliseconds
	RTT        time.Duration `json:"rtt"`
	Stratum    uint8         `json:"stratum"`
	Leap       uint8         `json:"leap"`
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
		TS     int64   `json:"ts"`
		Server string  `json:"server"`
		Offset float64 `json:"offset"` // todo: convert to float64 of milliseconds
		// RTT        float64 `json:"rtt"`
		Stratum    uint8 `json:"stratum"`
		Leap       uint8 `json:"leap"`
		NoResponse bool  `json:"no_response"`
	}{
		TS:         s.TS.Unix(),
		Server:     s.Server.String(),
		Offset:     float64(s.Offset) / float64(time.Second),
		Stratum:    s.Stratum,
		Leap:       s.Leap,
		NoResponse: s.NoResponse,
	})
}
