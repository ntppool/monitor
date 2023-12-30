package monitorv2

import (
	"net/netip"
)

func (cfg *GetConfigResponse) GetIP() *netip.Addr {
	return bytesToIP(cfg.GetIpBytes())
}

func (cfg *GetConfigResponse) GetNatIP() *netip.Addr {
	return bytesToIP(cfg.GetIpNatBytes())
}

func (s *Server) IP() *netip.Addr {
	return bytesToIP(s.IpBytes)
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
