package pb

import (
	"net/netip"

	"go.ntppool.org/monitor/logger"
)

// func (cfg *Config) IP() *netip.Addr {
// 	return cfg.GetIP()
// }

func (cfg *Config) GetIP() *netip.Addr {
	return bytesToIP(cfg.GetIPBytes())
}

func (cfg *Config) GetNatIP() *netip.Addr {
	return bytesToIP(cfg.GetIPNatBytes())
}

func (s *Server) IP() *netip.Addr {
	return bytesToIP(s.IPBytes)
}

func (ss *ServerStatus) SetIP(ip *netip.Addr) {
	b, err := ip.MarshalBinary()
	if err != nil {
		logger.Setup().Error("Could not set IP in ServerStatus", "ip", ip, "err", err)
		return
	}
	ss.IPBytes = b
}

func bytesToIP(b []byte) *netip.Addr {
	// log.Printf("ip bytes length: %d", len(b))
	if len(b) == 4 {
		var b4 [4]byte
		copy(b4[:], b)
		ip := netip.AddrFrom4(b4)
		return &ip
	}
	var b16 [16]byte
	copy(b16[:], b)
	ip := netip.AddrFrom16(b16)
	return &ip
}
