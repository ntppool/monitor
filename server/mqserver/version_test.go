package mqserver

import "testing"

func TestSupportsAdHocNTPCheck(t *testing.T) {
	tests := []struct {
		version  string
		expected bool
		desc     string
	}{
		{"v4.0.4", true, "exactly 4.0.4"},
		{"v4.0.5", true, "newer than 4.0.4"},
		{"v4.1.0", true, "newer major version"},
		{"v5.0.0", true, "much newer version"},
		{"v4.0.3", false, "older than 4.0.4"},
		{"v4.0.0", false, "much older than 4.0.4"},
		{"v3.8.6", true, "exactly 3.8.6"},
		{"v3.8.5", false, "older than 3.8.6"},
		{"v3.8.7", false, "newer than 3.8.6 but older than 4.0.4"},
		{"v3.9.0", false, "newer than 3.8.6 but older than 4.0.4"},
		{"v3.0.0", false, "much older version"},
		{"v2.0.0", false, "very old version"},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			result := supportsAdHocNTPCheck(test.version)
			if result != test.expected {
				t.Errorf("supportsAdHocNTPCheck(%q) = %v, expected %v", test.version, result, test.expected)
			}
		})
	}
}
