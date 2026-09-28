package main

import "testing"

func TestGoNames(t *testing.T) {
	for in, want := range map[string]string{"funnel": "Funnel", "columns-3": "Columns3", "circle-alert": "CircleAlert",
		"arrow-down-0-1": "ArrowDown01"} {
		if got := goName(in); got != want {
			t.Errorf("goName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestElements(t *testing.T) {
	for _, c := range []struct {
		tag   string
		attrs map[string]string
		want  string
	}{
		{"path", map[string]string{"d": "m9 9 6 6"}, "M9 9l6 6"},
		{"path", map[string]string{"d": "m9 9h6"}, "M9 9h6"},
		{"line", map[string]string{"x1": "12", "y1": "8", "x2": "12", "y2": "12.5"}, "M12 8L12 12.5"},
		{"polygon", map[string]string{"points": "1,2 3,4 5,0.5"}, "M1 2L3 4L5 .5Z"},
		{"circle", map[string]string{"cx": "12", "cy": "12", "r": "0.5"}, "M11.5 12a.5 .5 0 1 0 1 0a.5 .5 0 1 0-1 0Z"},
		{"rect", map[string]string{"x": "2", "y": "3", "width": "4", "height": "5"}, "M2 3h4v5h-4Z"},
	} {
		got, err := element(c.tag, c.attrs)
		if err != nil || got != c.want {
			t.Errorf("%s %v: %q, %v, want %q", c.tag, c.attrs, got, err, c.want)
		}
	}
	if _, err := element("path", map[string]string{"d": "M1 1", "stroke-width": "3"}); err == nil {
		t.Error("an attribute genicons does not know was taken")
	}
}

func TestReadNode(t *testing.T) {
	ic, err := readNode(`const __iconNode = [["path", { d: "M1 1h2", key: "a" }], ["circle", { cx: "5", cy: "5", r: ".5",
  fill: "currentColor", key: "b" }]];
const X = createLucideIcon("x", __iconNode);`)
	if err != nil || ic.path != "M1 1h2" || ic.fill != "M4.5 5a.5 .5 0 1 0 1 0a.5 .5 0 1 0-1 0Z" {
		t.Fatalf("read %+v, %v", ic, err)
	}
}
