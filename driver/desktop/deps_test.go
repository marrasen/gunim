package desktop

import (
	"os/exec"
	"strings"
	"testing"
)

// A program with no use for cryptography links none through this driver on Linux.
func TestTheLinuxDriverLinksNoCryptography(t *testing.T) {
	cmd := exec.CommandContext(t.Context(), "go", "list", "-deps", "github.com/marrasen/gunim/driver/desktop")
	cmd.Env = append(cmd.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, p := range strings.Fields(string(out)) {
		if p == "crypto" || strings.HasPrefix(p, "crypto/") {
			t.Errorf("the driver links %s", p)
		}
	}
}
