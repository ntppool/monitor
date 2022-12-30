package pb

import (
	"net/netip"
	"time"
)

func (ss *ServerStatus) AbsoluteOffset() *time.Duration {
	offset := ss.Offset.AsDuration()
	if offset < 0 {
		offset = offset * -1
	}
	return &offset
}

func (ss *ServerStatus) GetIP() *netip.Addr {
	return bytesToIP(ss.IPBytes)
}
