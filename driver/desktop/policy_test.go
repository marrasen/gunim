package desktop

import (
	"testing"
	"time"
)

func TestRedrawPolicyKeepsTheFasterWay(t *testing.T) {
	tests := []struct {
		name         string
		full, part   time.Duration
		wantFullOnly bool
	}{
		{"a GPU, where the copy costs more", 470 * time.Microsecond, 560 * time.Microsecond, true},
		{"software GL, where shading costs more", 3400 * time.Microsecond, 2300 * time.Microsecond, false},
	}
	for _, tt := range tests {
		var p redrawPolicy
		for range policySamples {
			p.add(false, tt.full)
			if p.decided {
				t.Fatalf("%s: decided with no partial frames timed", tt.name)
			}
		}
		for range policySamples {
			p.add(true, tt.part)
		}
		if !p.decided || p.fullOnly() != tt.wantFullOnly {
			t.Errorf("%s: decided %v, full only %v; want full only %v", tt.name, p.decided, p.fullOnly(), tt.wantFullOnly)
		}
	}
}

func TestRedrawPolicyStopsTimingWhenOneKindNeverComes(t *testing.T) {
	var p redrawPolicy
	for range policyTimed {
		p.add(false, time.Millisecond)
	}
	if !p.decided || p.fullOnly() {
		t.Fatalf("after %d full frames: decided %v, full only %v; want decided, redrawing in part", policyTimed, p.decided, p.fullOnly())
	}
}
