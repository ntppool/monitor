package scorer

import (
	"errors"
	"testing"
	"time"

	"go.ntppool.org/monitor/scorer/recentmedian"
)

func TestShouldSkipScoreError(t *testing.T) {
	now := time.Date(2026, 9, 11, 23, 13, 0, 0, time.UTC)

	tests := []struct {
		name string
		err  error
		ts   time.Time
		want bool
	}{
		{
			name: "no recent scores, recent entry",
			err:  recentmedian.ErrNoRecentScores,
			ts:   now.Add(-10 * time.Minute),
			want: true,
		},
		{
			name: "no recent scores, old entry",
			err:  recentmedian.ErrNoRecentScores,
			ts:   now.Add(-4 * time.Hour),
			want: true,
		},
		{
			name: "other error, recent entry",
			err:  errors.New("boom"),
			ts:   now.Add(-10 * time.Minute),
			want: false,
		},
		{
			name: "other error, old entry",
			err:  errors.New("boom"),
			ts:   now.Add(-4 * time.Hour),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkipScoreError(tt.err, tt.ts, now)
			if got != tt.want {
				t.Errorf("shouldSkipScoreError() = %v, want %v", got, tt.want)
			}
		})
	}
}
