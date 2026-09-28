// Command genicons writes gunim's icon package from Lucide's icons, as lucide-react ships them.
//
//	go run ./tools/genicons -src path/to/node_modules/lucide-react
//
// It writes icon/lucide.go, with a variable for each icon and alias, icon/byname/icons.go, with every icon by name,
// and copies Lucide's licence to icon/LICENSE-lucide.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

func main() {
	src := flag.String("src", "", "the lucide-react package directory")
	out := flag.String("out", ".", "the icon package directory")
	flag.Parse()
	if *src == "" {
		log.Fatal("genicons: -src is required")
	}
	if err := run(*src, *out); err != nil {
		log.Fatal(err)
	}
}

// icon is one Lucide icon: its name, and its strokes and fills as SVG path data.
type icon struct {
	name, path, fill string
}

// alias is an older name for an icon.
type alias struct{ name, target string }

func run(src, out string) error {
	version, err := readVersion(filepath.Join(src, "package.json"))
	if err != nil {
		return err
	}
	icons, aliases, err := readIcons(filepath.Join(src, "dist", "esm", "icons"))
	if err != nil {
		return err
	}
	lucide, byname, err := render(version, icons, aliases)
	if err != nil {
		return err
	}
	license, err := os.ReadFile(filepath.Join(src, "LICENSE"))
	if err != nil {
		return fmt.Errorf("genicons: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(out, "byname"), 0o755); err != nil {
		return fmt.Errorf("genicons: %w", err)
	}
	for name, data := range map[string][]byte{
		"lucide.go":       lucide,
		"byname/icons.go": byname,
		"LICENSE-lucide":  license,
	} {
		if err := os.WriteFile(filepath.Join(out, name), data, 0o644); err != nil {
			return fmt.Errorf("genicons: %w", err)
		}
	}
	return nil
}

// readVersion returns the version in a package.json.
func readVersion(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("genicons: %w", err)
	}
	var pkg struct{ Version string }
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", fmt.Errorf("genicons: %s: %w", path, err)
	}
	if pkg.Version == "" {
		return "", fmt.Errorf("genicons: %s has no version", path)
	}
	return pkg.Version, nil
}

var (
	reexport = regexp.MustCompile(`export \{ default \} from '\./([a-z0-9-]+)\.mjs';`)
	created  = regexp.MustCompile(`createLucideIcon\(\s*"([a-z0-9-]+)",\s*__iconNode\s*\)`)
)

// readIcons reads every icon and alias in lucide-react's icons directory.
func readIcons(dir string) ([]icon, []alias, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.mjs"))
	if err != nil {
		return nil, nil, fmt.Errorf("genicons: %w", err)
	}
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("genicons: no icons in %s", dir)
	}
	var (
		icons   []icon
		aliases []alias
	)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, fmt.Errorf("genicons: %w", err)
		}
		name := strings.TrimSuffix(filepath.Base(f), ".mjs")
		if name == "index" {
			continue
		}
		s := string(data)
		if m := reexport.FindStringSubmatch(s); m != nil {
			aliases = append(aliases, alias{name: name, target: m[1]})
			continue
		}
		m := created.FindStringSubmatch(s)
		if len(m) < 2 || m[1] != name {
			return nil, nil, fmt.Errorf("genicons: %s creates no icon named %s", f, name)
		}
		ic, err := readNode(s)
		if err != nil {
			return nil, nil, fmt.Errorf("genicons: %s: %w", f, err)
		}
		ic.name = name
		icons = append(icons, ic)
	}
	return icons, aliases, nil
}

// readNode reads the __iconNode array of an icon module into path data.
func readNode(s string) (icon, error) {
	const start = "const __iconNode = "
	i := strings.Index(s, start)
	if i < 0 {
		return icon{}, errors.New("no __iconNode")
	}
	lx := &lexer{s: s[i+len(start):]}
	var ic icon
	if err := lx.expect("["); err != nil {
		return icon{}, err
	}
	for lx.peek() != "]" {
		d, fill, err := lx.element()
		if err != nil {
			return icon{}, err
		}
		if fill {
			ic.fill += d
		} else {
			ic.path += d
		}
		if lx.peek() == "," {
			lx.next()
		}
	}
	return ic, nil
}

// lexer reads the JavaScript literals an icon module holds: arrays, objects with bare keys, and strings.
type lexer struct {
	s string
	i int
}

// element reads one element of an icon node, such as ["path", { d: "M1 1h2" }], as path data, and reports
// whether it is filled.
func (l *lexer) element() (d string, fill bool, err error) {
	var (
		tag   string
		attrs map[string]string
	)
	err = l.expect("[")
	if err == nil {
		tag, err = l.str()
	}
	if err == nil {
		err = l.expect(",")
	}
	if err == nil {
		attrs, err = l.object()
	}
	if err == nil {
		err = l.expect("]")
	}
	if err != nil {
		return "", false, err
	}
	d, err = element(tag, attrs)
	return d, attrs["fill"] != "" && attrs["fill"] != "none", err
}

// next returns the next token: a punctuation mark, a quoted string with its quotes, or a bare word.
func (l *lexer) next() string {
	for l.i < len(l.s) && strings.ContainsRune(" \t\r\n", rune(l.s[l.i])) {
		l.i++
	}
	if l.i >= len(l.s) {
		return ""
	}
	start := l.i
	switch c := l.s[l.i]; {
	case strings.ContainsRune("[]{},:", rune(c)):
		l.i++
	case c == '"':
		l.i++
		for l.i < len(l.s) && l.s[l.i] != '"' {
			if l.s[l.i] == '\\' {
				l.i++
			}
			l.i++
		}
		l.i++
	default:
		for l.i < len(l.s) && !strings.ContainsRune(" \t\r\n[]{},:\"", rune(l.s[l.i])) {
			l.i++
		}
	}
	return l.s[start:min(l.i, len(l.s))]
}

func (l *lexer) peek() string {
	i := l.i
	t := l.next()
	l.i = i
	return t
}

func (l *lexer) expect(tok string) error {
	if t := l.next(); t != tok {
		return fmt.Errorf("want %q, have %q", tok, t)
	}
	return nil
}

func (l *lexer) str() (string, error) {
	t := l.next()
	s, err := strconv.Unquote(t)
	if err != nil {
		return "", fmt.Errorf("want a string, have %q", t)
	}
	return s, nil
}

// object reads an object whose values are all strings.
func (l *lexer) object() (map[string]string, error) {
	if err := l.expect("{"); err != nil {
		return nil, err
	}
	m := map[string]string{}
	for l.peek() != "}" {
		key := l.next()
		if err := l.expect(":"); err != nil {
			return nil, err
		}
		v, err := l.str()
		if err != nil {
			return nil, err
		}
		m[key] = v
		if l.peek() == "," {
			l.next()
		}
	}
	l.next()
	return m, nil
}

// known are the attributes an element may carry, by element.
var known = map[string][]string{
	"path":     {"d"},
	"rect":     {"x", "y", "width", "height", "rx", "ry"},
	"circle":   {"cx", "cy", "r"},
	"ellipse":  {"cx", "cy", "rx", "ry"},
	"line":     {"x1", "y1", "x2", "y2"},
	"polyline": {"points"},
	"polygon":  {"points"},
}

// element returns an SVG element as path data that starts with an absolute moveto.
func element(tag string, attrs map[string]string) (string, error) {
	allowed, ok := known[tag]
	if !ok {
		return "", fmt.Errorf("unknown element %q", tag)
	}
	nums := map[string]float64{}
	for k, v := range attrs {
		if k == "key" || k == "fill" {
			continue
		}
		if !slices.Contains(allowed, k) {
			return "", fmt.Errorf("%s has an attribute %q genicons does not know", tag, k)
		}
		if k == "d" || k == "points" {
			continue
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return "", fmt.Errorf("%s %s: %w", tag, k, err)
		}
		nums[k] = f
	}
	var b pathBuilder
	switch tag {
	case "path":
		return absolute(attrs["d"])
	case "line":
		b.cmd('M', nums["x1"], nums["y1"])
		b.cmd('L', nums["x2"], nums["y2"])
	case "polyline", "polygon":
		f := strings.FieldsFunc(attrs["points"], func(r rune) bool { return r == ' ' || r == ',' })
		if len(f) < 4 || len(f)%2 != 0 {
			return "", fmt.Errorf("%s has points %q", tag, attrs["points"])
		}
		for i := 0; i < len(f); i += 2 {
			x, err1 := strconv.ParseFloat(f[i], 64)
			y, err2 := strconv.ParseFloat(f[i+1], 64)
			if err := errors.Join(err1, err2); err != nil {
				return "", fmt.Errorf("%s points: %w", tag, err)
			}
			b.cmd(map[bool]byte{true: 'M', false: 'L'}[i == 0], x, y)
		}
		if tag == "polygon" {
			b.cmd('Z')
		}
	case "circle":
		b.ellipse(nums["cx"], nums["cy"], nums["r"], nums["r"])
	case "ellipse":
		b.ellipse(nums["cx"], nums["cy"], nums["rx"], nums["ry"])
	case "rect":
		b.rect(nums)
	}
	return b.String(), nil
}

// absolute returns path data with a leading relative moveto made absolute, so it may follow other path data.
func absolute(d string) (string, error) {
	d = strings.TrimSpace(d)
	if d == "" || d[0] == 'M' {
		return d, nil
	}
	if d[0] != 'm' {
		return "", fmt.Errorf("path data %q starts with no moveto", d)
	}
	// The pairs after a leading relative moveto are relative linetos.
	m := regexp.MustCompile(`^m\s*([-+]?(?:\d+\.?\d*|\.\d+)(?:e[-+]?\d+)?)[\s,]*([-+]?(?:\d+\.?\d*|\.\d+)(?:e[-+]?\d+)?)`).
		FindStringSubmatchIndex(d)
	if m == nil {
		return "", fmt.Errorf("path data %q starts with a moveto genicons cannot read", d)
	}
	rest := strings.TrimLeft(d[m[1]:], " ,")
	out := "M" + d[m[2]:m[3]] + " " + d[m[4]:m[5]]
	if rest != "" && strings.ContainsRune("0123456789.-+", rune(rest[0])) {
		out += "l"
	}
	return out + rest, nil
}

// pathBuilder writes compact path data.
type pathBuilder struct{ b strings.Builder }

func (p *pathBuilder) String() string { return p.b.String() }

// cmd writes a command and its numbers.
func (p *pathBuilder) cmd(c byte, nums ...float64) {
	p.b.WriteByte(c)
	for i, n := range nums {
		s := strconv.FormatFloat(n, 'f', -1, 64)
		switch {
		case strings.HasPrefix(s, "0."):
			s = s[1:]
		case strings.HasPrefix(s, "-0."):
			s = "-" + s[2:]
		}
		if i > 0 && s[0] != '-' {
			p.b.WriteByte(' ')
		}
		p.b.WriteString(s)
	}
}

// ellipse writes an ellipse as two half arcs, starting at its left.
func (p *pathBuilder) ellipse(cx, cy, rx, ry float64) {
	p.cmd('M', cx-rx, cy)
	p.cmd('a', rx, ry, 0, 1, 0, 2*rx, 0)
	p.cmd('a', rx, ry, 0, 1, 0, -2*rx, 0)
	p.cmd('Z')
}

// rect writes a rectangle, its corners rounded as SVG rounds them.
func (p *pathBuilder) rect(n map[string]float64) {
	x, y, w, h := n["x"], n["y"], n["width"], n["height"]
	rx, okx := n["rx"]
	ry, oky := n["ry"]
	switch {
	case okx && !oky:
		ry = rx
	case oky && !okx:
		rx = ry
	}
	rx, ry = min(rx, w/2), min(ry, h/2)
	if rx <= 0 || ry <= 0 {
		p.cmd('M', x, y)
		p.cmd('h', w)
		p.cmd('v', h)
		p.cmd('h', -w)
		p.cmd('Z')
		return
	}
	p.cmd('M', x+rx, y)
	p.cmd('h', w-2*rx)
	p.cmd('a', rx, ry, 0, 0, 1, rx, ry)
	p.cmd('v', h-2*ry)
	p.cmd('a', rx, ry, 0, 0, 1, -rx, ry)
	p.cmd('h', -(w - 2*rx))
	p.cmd('a', rx, ry, 0, 0, 1, -rx, -ry)
	p.cmd('v', -(h - 2*ry))
	p.cmd('a', rx, ry, 0, 0, 1, rx, -ry)
	p.cmd('Z')
}

// goName returns a Lucide name in Go style, as lucide-react names its components: funnel is Funnel, and columns-3
// is Columns3.
func goName(name string) string {
	var b strings.Builder
	for part := range strings.SplitSeq(name, "-") {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return b.String()
}

// reserved are the names the icon package takes for itself.
var reserved = []string{"Icon", "Stroke", "LucideVersion"}

// render returns the source of icon/lucide.go and icon/byname/icons.go.
func render(version string, icons []icon, aliases []alias) (lucide, byname []byte, _ error) {
	slices.SortFunc(icons, func(a, b icon) int { return strings.Compare(a.name, b.name) })
	slices.SortFunc(aliases, func(a, b alias) int { return strings.Compare(a.name, b.name) })
	byName := map[string]icon{}
	taken := map[string]string{}
	for _, r := range reserved {
		taken[r] = "the icon package"
	}
	claim := func(name string) error {
		g := goName(name)
		if g == "" || g[0] < 'A' || g[0] > 'Z' {
			return fmt.Errorf("genicons: %q makes no Go name", name)
		}
		if other, ok := taken[g]; ok {
			return fmt.Errorf("genicons: %q and %q are both %s", name, other, g)
		}
		taken[g] = name
		return nil
	}
	for _, ic := range icons {
		if err := claim(ic.name); err != nil {
			return nil, nil, err
		}
		byName[ic.name] = ic
	}
	target := map[string]string{}
	for _, a := range aliases {
		target[a.name] = a.target
	}
	resolve := func(name string) (string, error) {
		for range 10 {
			if _, ok := byName[name]; ok {
				return name, nil
			}
			next, ok := target[name]
			if !ok {
				break
			}
			name = next
		}
		return "", fmt.Errorf("genicons: alias %q leads to no icon", name)
	}
	// vars are the aliases that need a variable: one whose Go name is its icon's, such as arrow-down-01 for
	// arrow-down-0-1, needs none.
	var vars []alias
	for _, a := range aliases {
		to, err := resolve(a.name)
		if err != nil {
			return nil, nil, err
		}
		if goName(a.name) == goName(to) {
			continue
		}
		if err := claim(a.name); err != nil {
			return nil, nil, err
		}
		vars = append(vars, alias{name: a.name, target: to})
	}

	header := fmt.Sprintf("// Code generated by genicons from lucide-react %s. DO NOT EDIT.\n\n", version)
	var l bytes.Buffer
	l.WriteString(header)
	l.WriteString("package icon\n\n")
	l.WriteString("// LucideVersion is the version of lucide-react the icons come from.\n")
	fmt.Fprintf(&l, "const LucideVersion = %q\n\n", version)
	for _, ic := range icons {
		fmt.Fprintf(&l, "// %s is Lucide's %s.\nvar %s = &Icon{Name: %q, Path: %q", goName(ic.name), ic.name,
			goName(ic.name), ic.name, ic.path)
		if ic.fill != "" {
			fmt.Fprintf(&l, ", Fill: %q", ic.fill)
		}
		l.WriteString("}\n\n")
	}
	for _, a := range vars {
		fmt.Fprintf(&l, "// %s is %s, by Lucide's older name %s.\nvar %s = %s\n\n", goName(a.name), goName(a.target),
			a.name, goName(a.name), goName(a.target))
	}

	var n bytes.Buffer
	n.WriteString(header)
	n.WriteString("package byname\n\nimport \"github.com/marrasen/gunim/icon\"\n\n")
	n.WriteString("// icons holds every icon by its Lucide name and its aliases.\n")
	n.WriteString("var icons = map[string]*icon.Icon{\n")
	all := make([]string, 0, len(icons)+len(aliases))
	for _, ic := range icons {
		all = append(all, ic.name)
	}
	for _, a := range aliases {
		all = append(all, a.name)
	}
	slices.Sort(all)
	for _, name := range all {
		fmt.Fprintf(&n, "\t%q: icon.%s,\n", name, goName(name))
	}
	n.WriteString("}\n")

	lucide, err := format.Source(l.Bytes())
	if err != nil {
		return nil, nil, fmt.Errorf("genicons: format lucide.go: %w", err)
	}
	byname, err = format.Source(n.Bytes())
	if err != nil {
		return nil, nil, fmt.Errorf("genicons: format icons.go: %w", err)
	}
	return lucide, byname, nil
}
