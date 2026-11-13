//go:build kmspi && linux

// The run on a real screen, as on the trial Raspberry Pi: it takes the
// screen, shows the test pattern moving with a touch script and then
// still, and checks the figures. No CI machine has a screen, so it runs
// only with the kmspi tag, as root for the VT and the backlight:
//
//	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c -tags kmspi -o kmspi.test ./driver/kms
//	sudo ./kmspi.test -test.v
//
// GUNIM_KMSPI_ROTATE turns the picture (90 by default), and GUNIM_KMSPI_VT
// names the VT ("free" by default). It skips where there is no screen to
// take.
package kms_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim/driver/kms"
	"github.com/marrasen/gunim/internal/kmsdemo"
)

func TestOnTheScreen(t *testing.T) {
	rotate := 90
	if v := os.Getenv("GUNIM_KMSPI_ROTATE"); v != "" {
		rotate, _ = strconv.Atoi(v)
	}
	vt := "free"
	if v, ok := os.LookupEnv("GUNIM_KMSPI_VT"); ok {
		vt = v
	}
	o := kmsdemo.BenchOptions{
		KMS:  kms.Options{Rotate: rotate, VT: vt},
		Warm: 2 * time.Second, Moving: 10 * time.Second, Rest: 5 * time.Second,
		Log: func(s string) { t.Log(s) },
	}
	r, err := kmsdemo.Bench(context.Background(), o)
	if err != nil && strings.Contains(err.Error(), "no connected screen") {
		t.Skipf("no screen to take: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range r.Goals() {
		t.Logf("%-24s %-20s %s", g.Name, g.Target, g.Got)
	}
	// What a run this short checks: the panel's rate while moving, frames
	// well inside a refresh, and nothing drawn at rest. The full goals are
	// kmsbench's.
	fps := float64(r.Moving.Frames) / r.MovingFor.Seconds()
	if fps < r.Rate*0.97 {
		t.Errorf("%.1f fps while moving, want the panel's %.2f Hz", fps, r.Rate)
	}
	if r.Moving.Frame.P99 > 8*time.Millisecond {
		t.Errorf("frame time p99 %v, want at most 8 ms", r.Moving.Frame.P99)
	}
	if r.Moving.Touch.N == 0 {
		t.Error("no touch from the script reached a frame")
	}
	if r.RestFrames != 0 {
		t.Errorf("%d frames drawn at rest, want none", r.RestFrames)
	}
}
