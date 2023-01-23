package api

import (
	"github.com/beevik/ntp"
)

type NTPResponse struct {
	Server string
	NTP    *ntp.Response
	Error  error
}
