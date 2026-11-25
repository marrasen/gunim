package gunim

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// escapeElsewhere are the functions of gunim's own that act on Escape with no [EscapeTaker], by package and receiver, and
// why Back is theirs all the same.
var escapeElsewhere = map[string]string{
	"widget.Editor":         "it leaves Escape to what it is in",
	"widget.editor":         "it leaves Escape to what it is in",
	"widget.TextArea":       "a popup that a press outside closes",
	"widget.Menu":           "a popup that a press outside closes",
	"widget.ContextMenu":    "a popup that a press outside closes",
	"widget.ContextArea":    "a popup that a press outside closes",
	"widget.Dropdown":       "a popup that a press outside closes",
	"widget.MenuButton":     "a popup that a press outside closes",
	"widget.emojiCard":      "a popup that a press outside closes",
	"widget.paletteCard":    "a popup that a press outside closes",
	"widget.ColorPicker":    "a popup that a press outside closes",
	"widget.NewColorPicker": "a popup that a press outside closes",
	"calendar.TimeField":    "a popup that a press outside closes",
	"calendar.eventKeys":    "Days and Month say so",
	"gunim.UI":              "the engine's own, for drags and Back",
}

// Every node of gunim's own that acts on Escape says so for a phone's Back: as an [EscapeTaker], a [BackTaker] or a
// [Modal], or by being in a popup.
func TestEveryNodeThatTakesEscapeSaysSo(t *testing.T) {
	type key struct{ dir, recv string }
	acts, says := map[key]string{}, map[key]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch {
			case path == "driver", path == "input", path == "internal", path == "example", path == "tools", path == ".git",
				strings.HasPrefix(d.Name(), "testdata"):
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, path, src, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			k := key{filepath.Dir(path), fn.Name.Name}
			if fn.Recv != nil {
				k.recv = recvName(fn.Recv.List[0].Type)
				switch fn.Name.Name {
				case "TakesEscape", "TakesBack", "Modal":
					says[k] = true
				}
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				if s, ok := n.(*ast.SelectorExpr); ok && s.Sel.Name == "KeyEscape" {
					acts[k] = f.Name.Name + "." + k.recv
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) < 20 {
		t.Fatalf("only %d nodes that act on Escape were found", len(acts))
	}
	for k, name := range acts {
		if !says[k] && escapeElsewhere[name] == "" {
			t.Errorf("%s acts on Escape, and has no TakesEscape, TakesBack or Modal", name)
		}
	}
}

// recvName returns the name of a receiver's type.
func recvName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.StarExpr:
		return recvName(e.X)
	case *ast.IndexExpr:
		return recvName(e.X)
	case *ast.IndexListExpr:
		return recvName(e.X)
	case *ast.Ident:
		return e.Name
	}
	return ""
}
