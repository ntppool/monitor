package monitor

import (
	"bytes"
	"net/netip"

	monitorv2 "go.ntppool.org/monitor/gen/monitor/v2"
)

type CaptureBuffer struct {
	dest, src *netip.Addr
	responses []*monitorv2.NTPPacket
}

func NewCaptureBuffer(dest, src *netip.Addr) *CaptureBuffer {
	return &CaptureBuffer{
		dest: dest,
		src:  src,
	}
}

func (cb *CaptureBuffer) ProcessQuery(buf *bytes.Buffer) error {
	return nil
}

func (cb *CaptureBuffer) ProcessResponse(buf []byte) error {
	pkt := monitorv2.NTPPacket{}
	pkt.Data = buf
	cb.responses = append(cb.responses, &pkt)
	return nil
}

func (cb *CaptureBuffer) ResponsePackets() []*monitorv2.NTPPacket {
	return cb.responses
}

func (cb *CaptureBuffer) Clear() {
	cb.responses = []*monitorv2.NTPPacket{}
}
