package pb

import (
	"time"

	"inet.af/netaddr"
)

func (ss *ServerStatus) AbsoluteOffset() *time.Duration {
	offset := ss.Offset.AsDuration()
	if offset < 0 {
		offset = offset * -1
	}
	return &offset
}

func (ss *ServerStatus) GetIP() *netaddr.IP {
	return bytesToIP(ss.IPBytes)
}
