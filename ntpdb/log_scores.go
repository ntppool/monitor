package ntpdb

import "time"

type LogScoreAttributes struct {
	Leap       int8   `json:"leap,omitempty"`
	Stratum    int8   `json:"stratum,omitempty"`
	NoResponse bool   `json:"no_response,omitempty"`
	Error      string `json:"error,omitempty"`
	Warning    string `json:"warning,omitempty"`

	FromLSID int `json:"from_ls_id,omitempty"`
	FromSSID int `json:"from_ss_id,omitempty"`
}

func (ls *LogScore) AbsoluteOffset() *time.Duration {
	if !ls.Offset.Valid {
		return nil
	}
	offset := time.Duration(ls.Offset.Float64 * float64(time.Second))
	if offset < 0 {
		offset = offset * -1
	}
	return &offset
}

func (ls *LogScore) MaxScore() (float64, bool) {
	offsetAbs := ls.AbsoluteOffset()
	if offsetAbs != nil && *offsetAbs > 3*time.Second {
		return -20, true
	}
	return 0, false
}
