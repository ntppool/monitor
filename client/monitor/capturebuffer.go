package monitor

import "bytes"

type CaptureBuffer struct {
	responses [][]byte
}

func (cb *CaptureBuffer) ProcessQuery(buf *bytes.Buffer) error {
	return nil
}

func (cb *CaptureBuffer) ProcessResponse(buf []byte) error {
	cb.responses = append(cb.responses, buf)
	return nil
}

func (cb *CaptureBuffer) ResponsePackets() [][]byte {
	return cb.responses
}

func (cb *CaptureBuffer) Clear() {
	cb.responses = [][]byte{}
}
