package monitor

// // ServerList has a list of servers to check and the Config to use
// type ServerList struct {
// 	Config  *Config  `json:"config"`
// 	Servers []string `json:"servers"`
// }

// ServerStatus ...
// type ServerStatus struct {
// 	TS         time.Time     `json:"ts"`
// 	Server     *net.IP       `json:"server"`
// 	Offset     time.Duration `json:"offset"`
// 	RTT        time.Duration `json:"rtt,omitempty"`
// 	Stratum    uint8         `json:"stratum"`
// 	Leap       uint8         `json:"leap"`
// 	Error      string        `json:"error,omitempty"`
// 	NoResponse bool          `json:"no_response"`
// }

// Feedback ...
// type Feedback struct {
// 	Version int             `json:"version"`
// 	Servers []*ServerStatus `json:"servers"`
// }

// MarshalJSON encodes ServerStatus to JSON as expected by
// the pool API
// func (s *ServerStatus) MarshalJSON() ([]byte, error) {
// 	if s.TS.IsZero() {
// 		return nil, fmt.Errorf("TS is zero")
// 	}
// 	return json.Marshal(&struct {
// 		TS         int64   `json:"ts"`
// 		Server     string  `json:"server"`
// 		Offset     float64 `json:"offset"`
// 		RTT        int64   `json:"rtt,omitempty"`
// 		Stratum    uint8   `json:"stratum"`
// 		Leap       uint8   `json:"leap"`
// 		Error      string  `json:"error,omitempty"`
// 		NoResponse bool    `json:"no_response"`
// 	}{
// 		TS:         s.TS.Unix(),
// 		Server:     s.Server.String(),
// 		Offset:     float64(s.Offset) / float64(time.Second),
// 		RTT:        int64(s.RTT / time.Microsecond),
// 		Stratum:    s.Stratum,
// 		Leap:       s.Leap,
// 		Error:      s.Error,
// 		NoResponse: s.NoResponse,
// 	})
// }
