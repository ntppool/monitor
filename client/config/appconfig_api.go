package config

type MonitorStatus struct {
	Status string `json:"status"`
	IP     string `json:"ip"`
}

type MonitorStatusConfig struct {
	Name    string        `json:"name"`
	TLSName string        `json:"tls_name"`
	IPv4    MonitorStatus `json:"ipv4"`
	IPv6    MonitorStatus `json:"ipv6"`

	TLS struct {
		Key  string `json:"key"`
		Cert string `json:"cert"`
	} `json:"tls"`
}
