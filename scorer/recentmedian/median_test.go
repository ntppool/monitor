package recentmedian

import (
	"errors"
	"testing"
)

func TestNoRecentScoresError_WrapsSentinel(t *testing.T) {
	err := errNoRecentScores(66636)

	if !errors.Is(err, ErrNoRecentScores) {
		t.Fatalf("expected error to wrap ErrNoRecentScores, got %v", err)
	}

	want := "no recent scores found for 66636"
	if err.Error() != want {
		t.Errorf("error message = %q, want %q", err.Error(), want)
	}
}
