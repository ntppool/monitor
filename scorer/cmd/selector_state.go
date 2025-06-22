package cmd

//go:generate go tool github.com/dmarkham/enumer -type=candidateState

import (
	"go.ntppool.org/monitor/ntpdb"
)

// candidateState represents the recommended action for a monitor
type candidateState uint8

const (
	candidateUnknown candidateState = iota
	candidateIn                    // Should be promoted/kept active
	candidateOut                   // Should be demoted (gradual)
	candidateBlock                 // Should be removed immediately
)

// IsOutOfOrder returns the "most out of order" of the currently active monitors.
// The second return parameter is the ID of the better monitor candidate,
// the first return parameter the ID to be replaced. The last parameter
// is false if no relevant replacement was found.
func (nsl newStatusList) IsOutOfOrder() (uint32, uint32, bool) {
	best := uint32(0)
	replace := uint32(0)

	for _, ns := range nsl {
		if ns.NewState != candidateIn {
			continue
		}
		switch ns.CurrentStatus {
		case ntpdb.ServerScoresStatusActive:
			// only replace if we found a replacement
			if best != 0 {
				replace = ns.MonitorID
			}

		case ntpdb.ServerScoresStatusTesting:
			if best == 0 {
				best = ns.MonitorID
			}

		}

	}

	if best == 0 || replace == 0 {
		return 0, 0, false
	}

	return best, replace, true
}
