package pb

import (
	"log"

	"inet.af/netaddr"
)

// func (cfg *Config) IP() *netaddr.IP {
// 	return cfg.GetIP()
// }

func (cfg *Config) GetIP() *netaddr.IP {
	return bytesToIP(cfg.GetIPBytes())
}

func (cfg *Config) GetNatIP() *netaddr.IP {
	return bytesToIP(cfg.GetIPNatBytes())
}

func (s *Server) IP() *netaddr.IP {
	return bytesToIP(s.IPBytes)
}

func (ss *ServerStatus) SetIP(ip *netaddr.IP) {
	b, err := ip.MarshalBinary()
	if err != nil {
		log.Printf("Could not set IP %s in ServerStatus: %s ", ip, err)
		return
	}
	ss.IPBytes = b
}

func bytesToIP(b []byte) *netaddr.IP {
	// log.Printf("ip bytes length: %d", len(b))
	if len(b) == 4 {
		ip := netaddr.IPv4(b[0], b[1], b[2], b[3])
		return &ip
	}
	var b16 [16]byte
	copy(b16[:], b)
	ip := netaddr.IPFrom16(b16)
	return &ip
}
