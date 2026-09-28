package widget

import (
	"testing"

	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/icon"
)

func TestTabsShowAnIconBeforeEachTitle(t *testing.T) {
	plain := NewTabs([]string{"Files", "Search"}, NewLabel("a"), NewLabel("b"))
	tabs := NewTabs([]string{"Files", "Search"}, NewLabel("a"), NewLabel("b"))
	tabs.Icons = []*icon.Icon{icon.Folder, icon.Search}
	stage(t, Row(plain))
	w, run := stage(t, Row(tabs))
	run(2)
	ms := maskOps(w.Offscreen())
	if len(ms) != 2 || strokeOf(t, ms[0]).Icon != icon.Folder || strokeOf(t, ms[1]).Icon != icon.Search {
		t.Fatalf("the tabs drew %d icons, want a folder and a search", len(ms))
	}
	room := IconSize.Default() + IconGap.Default()
	for i := range 2 {
		wide := tabs.spans[i][1] - tabs.spans[i][0]
		was := plain.spans[i][1] - plain.spans[i][0]
		if wide-was != room {
			t.Errorf("tab %d is %v wider with its icon, want %v", i, wide-was, room)
		}
		if x := ms[i].Rect.Min.X; x != tabs.spans[i][0]+TabPadding.Default() {
			t.Errorf("tab %d's icon is at %v, want at its padding", i, x)
		}
	}
	if ms[0].Color != Ink.Default() || ms[1].Color != Placeholder.Default() {
		t.Fatalf("the icons are %v and %v, want the chosen one in ink and the other dim", ms[0].Color, ms[1].Color)
	}
	click(w, tabs.spans[1][0]+room+10, tabs.head/2)
	run(30)
	ms = maskOps(w.Offscreen())
	if tabs.Selected() != 1 || ms[0].Color != Placeholder.Default() || ms[1].Color != Ink.Default() {
		t.Fatalf("after choosing the second tab its icons are %v and %v", ms[0].Color, ms[1].Color)
	}
	info := tabs.bar.Access()
	if len(info.Parts) != 2 || info.Parts[0].Name != "Files" || info.Parts[1].Name != "Search" ||
		info.Parts[1].Role != access.RoleTab {
		t.Fatalf("the tabs read as %+v, want their titles", info.Parts)
	}
	if r := info.Parts[1].Bounds; r.Min.X != tabs.spans[1][0] || r.Size().W != tabs.spans[1][1]-tabs.spans[1][0] {
		t.Fatalf("the second tab reads as at %v, want its whole span with the icon", r)
	}
}
