package api

import (
	"github.com/beevik/ntp"
)

type NTPResponse struct {
	Server string        `json:",omitempty"`
	NTP    *ntp.Response `json:",omitempty"`
	Error  string        `json:",omitempty"`
}
