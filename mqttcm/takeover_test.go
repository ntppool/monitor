package mqttcm

import (
	"context"
	"testing"
	"time"

	"github.com/eclipse/paho.golang/packets"
)

func TestReconnectDelaySchedule(t *testing.T) {
	s := newTakeoverState(time.Hour) // value doesn't matter for this test

	if got, want := s.reconnectDelay(), defaultReconnectDelay; got != want {
		t.Errorf("initial reconnectDelay = %v, want %v", got, want)
	}

	steps := []time.Duration{
		2 * time.Minute,
		5 * time.Minute,
		10 * time.Minute,
		maxReconnectBackoff,
		maxReconnectBackoff,
	}
	for i, want := range steps {
		s.recordDisconnect(packets.DisconnectSessionTakenOver)
		if got := s.reconnectDelay(); got != want {
			t.Errorf("after takeover #%d: reconnectDelay = %v, want %v", i+1, got, want)
		}
	}
}

func TestInBackoffTransitions(t *testing.T) {
	s := newTakeoverState(time.Hour)

	if s.InBackoff() {
		t.Fatal("fresh TakeoverState should not be in backoff")
	}

	s.recordDisconnect(packets.DisconnectSessionTakenOver)
	if !s.InBackoff() {
		t.Fatal("takeover disconnect should put state in backoff")
	}
}

func TestWaitRecoveredImmediateWhenNotInBackoff(t *testing.T) {
	s := newTakeoverState(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := s.WaitRecovered(ctx); err != nil {
		t.Fatalf("WaitRecovered: %v", err)
	}
}

func TestWaitRecoveredReleasesOnStability(t *testing.T) {
	s := newTakeoverState(20 * time.Millisecond)
	s.recordDisconnect(packets.DisconnectSessionTakenOver)
	s.markConnectionUp() // will fire resetAfterStability ~20ms later

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	start := time.Now()
	if err := s.WaitRecovered(ctx); err != nil {
		t.Fatalf("WaitRecovered: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 15*time.Millisecond {
		t.Errorf("WaitRecovered returned too quickly: %v", elapsed)
	}
	if s.InBackoff() {
		t.Error("should not be in backoff after stability window")
	}
	if got, want := s.reconnectDelay(), defaultReconnectDelay; got != want {
		t.Errorf("post-recovery reconnectDelay = %v, want %v", got, want)
	}
}

func TestWaitRecoveredHonoursContextCancel(t *testing.T) {
	s := newTakeoverState(time.Hour) // never fires in test
	s.recordDisconnect(packets.DisconnectSessionTakenOver)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := s.WaitRecovered(ctx)
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
	if !s.InBackoff() {
		t.Error("context cancel must not clear backoff state")
	}
}

func TestShortReconnectDoesNotRelease(t *testing.T) {
	// Connect-up followed by another takeover before stabilityWindow elapses
	// must keep the counter > 0 and must not release waiters.
	s := newTakeoverState(50 * time.Millisecond)

	s.recordDisconnect(packets.DisconnectSessionTakenOver) // count = 1
	s.markConnectionUp()                                   // schedule 50ms stability timer
	time.Sleep(5 * time.Millisecond)
	s.recordDisconnect(packets.DisconnectSessionTakenOver) // cancel timer, count = 2

	// The first scheduled timer should not fire after being cancelled.
	time.Sleep(80 * time.Millisecond)

	if !s.InBackoff() {
		t.Fatal("should still be in backoff after short reconnect")
	}
	if got, want := s.reconnectDelay(), 5*time.Minute; got != want {
		t.Errorf("reconnectDelay after two takeovers = %v, want %v", got, want)
	}
}

func TestNonTakeoverDisconnectDoesNotIncrementButStopsTimer(t *testing.T) {
	s := newTakeoverState(20 * time.Millisecond)

	s.recordDisconnect(packets.DisconnectSessionTakenOver) // count = 1, backoff
	s.markConnectionUp()                                   // arm stability timer
	time.Sleep(5 * time.Millisecond)
	s.recordDisconnect(0x00) // generic disconnect: does not bump count, cancels timer

	if got, want := s.reconnectDelay(), 2*time.Minute; got != want {
		t.Errorf("reconnectDelay = %v, want %v (takeover count should be unchanged)", got, want)
	}

	// Timer must have been cancelled: waiting past stabilityWindow should not
	// clear backoff, because without a fresh markConnectionUp no timer is armed.
	time.Sleep(40 * time.Millisecond)
	if !s.InBackoff() {
		t.Error("non-takeover disconnect must cancel stability timer")
	}
}

func TestWaitersReleasedOnlyOnce(t *testing.T) {
	s := newTakeoverState(10 * time.Millisecond)
	s.recordDisconnect(packets.DisconnectSessionTakenOver)
	s.markConnectionUp()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.WaitRecovered(ctx); err != nil {
		t.Fatalf("first WaitRecovered: %v", err)
	}

	// A fresh takeover after recovery must create a new (open) channel so the
	// next WaitRecovered blocks rather than returning instantly from a stale
	// closed channel reference.
	s.recordDisconnect(packets.DisconnectSessionTakenOver)
	if !s.InBackoff() {
		t.Fatal("expected backoff after new takeover")
	}
	quick, cancel2 := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel2()
	if err := s.WaitRecovered(quick); err == nil {
		t.Error("WaitRecovered returned nil while in backoff; expected context error")
	}
}

func TestStabilityWindowInvariant(t *testing.T) {
	// The plan depends on stabilityWindow > maxReconnectBackoff so a cyclic
	// intruder whose period falls in between cannot let us reset the counter
	// between its reappearances. Guard this with a test so a tuning change
	// does not silently regress the duplicate-submission guarantee.
	if defaultStabilityWindow <= maxReconnectBackoff {
		t.Fatalf("defaultStabilityWindow (%v) must exceed maxReconnectBackoff (%v)",
			defaultStabilityWindow, maxReconnectBackoff)
	}
}
