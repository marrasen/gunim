package widget

import (
	"os"
	"testing"

	"github.com/marrasen/gunim"
)

// The widgets' tests name every call about a node that is not in the
// tree.
func TestMain(m *testing.M) {
	gunim.PanicOnStrays(true)
	os.Exit(m.Run())
}
