package vst3

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tests run LSP's plugins, where they are: LSP_VST3 names the bundle.
func lsp(t *testing.T) *Module {
	t.Helper()
	path := os.Getenv("LSP_VST3")
	if path == "" {
		path = "/tmp/lsp/lsp-plugins-1.2.35-Linux-x86_64/VST3/lsp-plugins.vst3"
	}
	if _, err := os.Stat(path); err != nil {
		t.Skip("no LSP plugins at", path)
	}
	m, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func class(t *testing.T, m *Module, name string) Class {
	t.Helper()
	for _, c := range m.Effects() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %q among %d effects", name, len(m.Effects()))
	return Class{}
}

// sine returns n stereo frames of a sine at 1 kHz, at amplitude a.
func sine(n int, a float64) []float32 {
	out := make([]float32, 2*n)
	for i := range n {
		v := float32(a * math.Sin(2*math.Pi*1000*float64(i)/44100))
		out[2*i], out[2*i+1] = v, v
	}
	return out
}

func rms(b []float32) float64 {
	var s float64
	for _, v := range b {
		s += float64(v) * float64(v)
	}
	return math.Sqrt(s / float64(len(b)))
}

func param(t *testing.T, p *Plugin, title string) Param {
	t.Helper()
	for _, q := range p.Params() {
		if strings.EqualFold(q.Title, title) {
			return q
		}
	}
	var names []string
	for _, q := range p.Params() {
		names = append(names, q.Title)
	}
	t.Fatalf("no parameter %q: %v", title, names)
	return Param{}
}

func TestAPluginProcessesAndRestoresItsState(t *testing.T) {
	m := lsp(t)
	c := class(t, m, "Parametric Equalizer x16 Stereo")
	p, err := m.New(c, Config{Rate: 44100})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	in := sine(44100, 0.5)
	out := append([]float32(nil), in...)
	p.Process(out)
	tail := func(b []float32) float64 { return rms(b[len(b)/2:]) }
	if r := tail(out) / tail(in); math.Abs(r-1) > 0.05 {
		t.Fatalf("at its defaults, the equalizer changed the level by %.3f", r)
	}
	t.Logf("single %v, connected %v", p.single, p.compCP != 0 && p.ctrlCP != 0)
	// Down goes the output gain, and the level with it.
	g := param(t, p, "Output gain")
	p.Set(g.ID, g.Default/2)
	quieter := append([]float32(nil), in...)
	p.Process(quieter)
	if r := tail(quieter) / tail(in); r > 0.9 {
		t.Fatalf("with the output gain down, the level is %.3f of the input's", r)
	}
	st, err := p.State()
	if err != nil {
		t.Fatal(err)
	}
	// A second equalizer, of the first's state, sounds as it does.
	q, err := m.New(c, Config{Rate: 44100, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	if err := q.SetState(st); err != nil {
		t.Fatal(err)
	}
	again := append([]float32(nil), in...)
	q.Process(again)
	if a, b := tail(again), tail(quieter); math.Abs(a-b) > 0.01*b {
		t.Fatalf("restored, the equalizer plays at %.4f, the first at %.4f", a, b)
	}
}

func TestAPluginOfASidechainProcesses(t *testing.T) {
	m := lsp(t)
	p, err := m.New(class(t, m, "Sidechain Compressor Stereo"), Config{Rate: 44100, Block: 512})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	in := sine(44100, 0.9)
	out := append([]float32(nil), in...)
	p.Process(out)
	if r := rms(out[len(out)/2:]); r == 0 || math.IsNaN(r) {
		t.Fatalf("the compressor played %v", r)
	}
	t.Logf("buses in %d out %d, level %.3f of %.3f", len(p.ins), len(p.outs), rms(out[len(out)/2:]), rms(in))
}

func TestABundleListsTheEffectsItsModuleMakes(t *testing.T) {
	m := lsp(t)
	listed := moduleInfo(m.Path)
	made := m.Effects()
	if len(listed) == 0 || len(listed) != len(made) {
		t.Fatalf("the bundle lists %d effects, the module makes %d", len(listed), len(made))
	}
	names := map[uid]string{}
	for _, c := range made {
		names[c.ID] = c.Name
	}
	for _, c := range listed {
		if names[c.ID] != c.Name {
			t.Fatalf("listed %s as %x, which the module makes as %q", c.Name, c.ID, names[c.ID])
		}
	}
	if b := Scan(filepath.Dir(m.Path)); len(b) != 1 || b[0].Name != "lsp-plugins" {
		t.Fatalf("the scan found %v", b)
	}
}
