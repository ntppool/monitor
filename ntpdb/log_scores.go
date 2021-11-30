package ntpdb

type LogScoreAttributes struct {
	Leap    int8   `json:"leap,omitempty"`
	Error   string `json:"error,omitempty"`
	Warning string `json:"warning,omitempty"`
}
