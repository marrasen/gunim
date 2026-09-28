package gunim

import (
	"os"
	"testing"
)

// gunim's tests name every call about a node that is not in the tree.
func TestMain(m *testing.M) {
	PanicOnStrays(true)
	os.Exit(m.Run())
}
