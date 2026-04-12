package mqttcm

import (
	"context"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/packets"
)

const (
	defaultReconnectDelay  = 10 * time.Second
	maxReconnectBackoff    = 15 * time.Minute
	defaultStabilityWindow = 30 * time.Minute
)

// stabilityWindow must exceed maxReconnectBackoff; otherwise a cyclic intruder
// whose period falls in between could let the takeover counter reset between
// its reappearances, causing us to resume real work during a genuine conflict.
// The sizing invariant is asserted in tests.

// TakeoverState tracks whether we believe another MQTT client has taken over
// our session. Callers use InBackoff / WaitRecovered to gate real work (NTP
// checks, result submission) while the conflict is active.
//
// The zero value is not usable; construct with NewTakeoverState.
type TakeoverState struct {
	stabilityWindow time.Duration

	mu             sync.Mutex
	takeoverCount  int
	recoveredCh    chan struct{} // nil when takeoverCount == 0
	stabilityTimer *time.Timer
}

func newTakeoverState(stabilityWindow time.Duration) *TakeoverState {
	return &TakeoverState{stabilityWindow: stabilityWindow}
}

// NewTakeoverState returns a TakeoverState with the production stabilityWindow.
func NewTakeoverState() *TakeoverState {
	return newTakeoverState(defaultStabilityWindow)
}

// InBackoff reports whether we currently believe another client holds our
// session. Safe for concurrent use.
func (s *TakeoverState) InBackoff() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.takeoverCount > 0
}

// WaitRecovered blocks until the takeover backoff clears or ctx is done.
// Returns nil on recovery (or if not currently in backoff) and ctx.Err() on
// cancellation.
func (s *TakeoverState) WaitRecovered(ctx context.Context) error {
	s.mu.Lock()
	ch := s.recoveredCh
	s.mu.Unlock()
	if ch == nil {
		return nil
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// recordDisconnect updates state for a broker-initiated DISCONNECT. Any
// pending stability timer is stopped: we need another clean connect-up to
// start measuring stability again.
func (s *TakeoverState) recordDisconnect(reasonCode byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stabilityTimer != nil {
		s.stabilityTimer.Stop()
		s.stabilityTimer = nil
	}

	if reasonCode != packets.DisconnectSessionTakenOver {
		return
	}

	if s.takeoverCount == 0 {
		s.recoveredCh = make(chan struct{})
	}
	s.takeoverCount++
}

// markConnectionUp arms the stability timer when we are in takeover backoff.
// If the connection holds for stabilityWindow, the counter is cleared and
// waiters are released. A no-op when not in backoff.
func (s *TakeoverState) markConnectionUp() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.takeoverCount == 0 {
		return
	}
	if s.stabilityTimer != nil {
		s.stabilityTimer.Stop()
	}
	s.stabilityTimer = time.AfterFunc(s.stabilityWindow, s.resetAfterStability)
}

func (s *TakeoverState) resetAfterStability() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.takeoverCount == 0 {
		return
	}
	s.takeoverCount = 0
	close(s.recoveredCh)
	s.recoveredCh = nil
	s.stabilityTimer = nil
}

// reconnectDelay returns the backoff duration before the next connection
// attempt. The schedule is deliberately floor-heavy (10s → 2m is a 12x jump)
// rather than smoothly exponential: on a takeover we want to get out of the
// broker's way quickly, not iterate through tiny retries.
func (s *TakeoverState) reconnectDelay() time.Duration {
	s.mu.Lock()
	n := s.takeoverCount
	s.mu.Unlock()

	switch n {
	case 0:
		return defaultReconnectDelay
	case 1:
		return 2 * time.Minute
	case 2:
		return 5 * time.Minute
	case 3:
		return 10 * time.Minute
	default:
		return maxReconnectBackoff
	}
}
