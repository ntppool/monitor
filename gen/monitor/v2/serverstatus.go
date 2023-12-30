package monitorv2

import (
	"net/netip"
	"time"

	"go.ntppool.org/common/logger"
)

func (ss *ServerStatus) AbsoluteOffset() *time.Duration {
	offset := ss.Offset.AsDuration()
	if offset < 0 {
		offset = offset * -1
	}
	return &offset
}

func (ss *ServerStatus) SetIP(ip *netip.Addr) {
	b, err := ip.MarshalBinary()
	if err != nil {
		logger.Setup().Error("Could not set IP in ServerStatus", "ip", ip, "err", err)
		return
	}
	ss.IpBytes = b
}

func (ss *ServerStatus) GetIP() *netip.Addr {
	return bytesToIP(ss.IpBytes)
}
