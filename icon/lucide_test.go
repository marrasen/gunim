package icon_test

import (
	"testing"

	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/icon/byname"
)

func TestEveryIconParsesAndDraws(t *testing.T) {
	names := byname.Names()
	if len(names) < 2000 {
		t.Fatalf("byname knows %d names, want Lucide's 2000 or so", len(names))
	}
	for _, name := range names {
		ic, ok := byname.Lookup(name)
		if !ok {
			t.Fatalf("%s is listed and not found", name)
		}
		if err := ic.Check(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		drawn := false
		for _, c := range ic.Stroke().Coverage(16, 16) {
			drawn = drawn || c > 0
		}
		if !drawn {
			t.Errorf("%s draws nothing at 16 pixels", name)
		}
	}
}

func TestAliasesResolve(t *testing.T) {
	for _, c := range []struct {
		alias, to *icon.Icon
		name      string
	}{
		{icon.Filter, icon.Funnel, "filter"},
		{icon.AlertTriangle, icon.TriangleAlert, "alert-triangle"},
		{icon.Loader2, icon.LoaderCircle, "loader-2"},
	} {
		if c.alias != c.to {
			t.Errorf("%s is %s, want %s", c.name, c.alias.Name, c.to.Name)
		}
		if ic, _ := byname.Lookup(c.name); ic != c.to {
			t.Errorf("looking up %s found %v, want %s", c.name, ic, c.to.Name)
		}
	}
	if icon.LucideVersion != "1.31.0" {
		t.Errorf("the icons come from lucide-react %s", icon.LucideVersion)
	}
}

func TestElementsBecomePathData(t *testing.T) {
	for _, c := range []struct {
		ic   *icon.Icon
		path string
	}{
		{icon.Funnel, "M10 20a1 1 0 0 0 .553.895l2 1A1 1 0 0 0 14 21v-7a2 2 0 0 1 .517-1.341L21.74 4.67A1 1 0 0 0 21 3H3a1 " +
			"1 0 0 0-.742 1.67l7.225 7.989A2 2 0 0 1 10 14z"},
		{icon.Circle, "M2 12a10 10 0 1 0 20 0a10 10 0 1 0-20 0Z"},
		{icon.Square, "M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-14a2 2 0 0 1-2-2v-14a2 2 0 0 1 2-2Z"},
		{icon.CircleAlert, "M2 12a10 10 0 1 0 20 0a10 10 0 1 0-20 0ZM12 8L12 12M12 16L12.01 16"},
	} {
		if c.ic.Path != c.path {
			t.Errorf("%s is\n%q, want\n%q", c.ic.Name, c.ic.Path, c.path)
		}
	}
	if icon.ChartScatter.Fill == "" {
		t.Error("chart-scatter's filled dots are missing")
	}
}
