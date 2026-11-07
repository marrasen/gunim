//go:build js && wasm

package web

import (
	"encoding/json"
	"fmt"
	"strings"
	"syscall/js"
	"testing"
	"time"
)

// AXNode is a node of Chrome's own accessibility tree.
type AXNode struct {
	Ignored     bool
	Role        struct{ Value string }
	Name        struct{ Value string }
	Value       struct{ Value any }
	Description struct{ Value string }
	Properties  []struct {
		Name  string
		Value struct {
			Value        any
			RelatedNodes []struct{ Idref string }
		}
	}
}

// Prop returns a property of the node, written out, and whether Chrome shows it.
func (n *AXNode) Prop(name string) (string, bool) {
	for _, p := range n.Properties {
		if p.Name == name {
			if len(p.Value.RelatedNodes) > 0 {
				return p.Value.RelatedNodes[0].Idref, true
			}
			return fmt.Sprint(p.Value.Value), true
		}
	}
	return "", false
}

// AXTree is Chrome's accessibility tree of the page, as a screen reader gets it.
type AXTree []AXNode

// Find returns the node of a role whose name starts with name, or nil.
func (t AXTree) Find(role, name string) *AXNode {
	for i := range t {
		n := &t[i]
		if !n.Ignored && n.Role.Value == role && strings.HasPrefix(n.Name.Value, name) {
			return n
		}
	}
	return nil
}

// AskAX reads Chrome's accessibility tree. The page has no way to read it, so it asks
// tools/webtest, which speaks the debugging protocol. Run another way, the test skips.
func AskAX(t *testing.T) AXTree {
	t.Helper()
	win := js.Global()
	if !win.Get("axRunner").Truthy() {
		t.Skip("no debugging protocol to read the accessibility tree: run this with go run ./tools/webtest")
	}
	win.Set("axTree", js.Undefined())
	win.Set("axAsked", true)
	for give := time.Now().Add(60 * time.Second); win.Get("axTree").IsUndefined(); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(give) {
			t.Fatal("tools/webtest never answered with the accessibility tree")
		}
	}
	var answer struct{ Nodes AXTree }
	if err := json.Unmarshal([]byte(win.Get("axTree").String()), &answer); err != nil {
		t.Fatal(err)
	}
	return answer.Nodes
}
