package chat

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
)

// The box round the composer says it takes Escape while there is a reply or an edit to stop, for a phone's Back.
func TestAComposerWithAReplyTakesEscape(t *testing.T) {
	for name, c := range map[string]struct {
		reply *replyBar
		want  bool
	}{
		"none": {nil, false}, "shut": {&replyBar{open: anim.NewFloat(0)}, false}, "open": {&replyBar{open: anim.NewFloat(1)}, true},
	} {
		if got := gunim.EscapeTaker(&composerBox{reply: c.reply}).TakesEscape(nil); got != c.want {
			t.Errorf("with a reply %s, it takes Escape: %v", name, got)
		}
	}
}
