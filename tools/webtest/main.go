// Command webtest runs the tests that need a real browser in one: driver/web's,
// and those of the package text for a browser. It exits with their result.
//
//	go run ./tools/webtest
//
// It builds the tests for js, serves them, opens the page in headless Chrome and
// prints what the tests print. Chrome is told its page has window focus, as some
// tests need that, and a test that skips fails the run, so a pass means all of them
// ran. It talks to Chrome over --remote-debugging-pipe, which takes no WebSocket
// client, and that pipe is not there on Windows.
//
// With -serve it builds and serves the page and stops there, to open in any other
// browser or on a phone. The result is then on the page, and in window.testResult.
//
// A test can ask the runner, through window.webtest, for what only the browser's
// user can do: a click, which lets the page show a file picker, and the files to
// choose in the picker it shows next. Under -serve there is no window.webtest, and
// such tests skip.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/marrasen/gunim/driver/web/saveworker"
)

// page runs the tests and keeps what they print in window.testResult. It
// names gunim's save worker, served beside it.
const page = `<!doctype html><meta charset="utf-8"><title>gunim tests</title>
<meta name="gunim-save-worker" content="gunim-save.js">
<canvas id="gunim" style="width:320px;height:240px"></canvas><pre id="output" style="white-space:pre-wrap"></pre>
<script src="wasm_exec.js"></script><script>
window.testResult = {done: false, code: null, output: "", read: 0};
const decoder = new TextDecoder(), write = globalThis.fs.writeSync;
const add = s => { testResult.output += s; document.querySelector("#output").textContent += s; };
globalThis.fs.writeSync = (fd, data) => { add(decoder.decode(data)); return write(fd, data); };
const go = new Go();
const query = new URLSearchParams(location.search), of = query.get("of") || "web", only = query.get("run");
go.argv = [of + ".test", "-test.v"].concat(only ? ["-test.run=" + only] : of == "text" ? ["-test.run=^TestBrowser"] : []);
go.exit = code => { testResult.code = code; testResult.done = true; };
if (typeof webtestAsk == "function") {
  const waiting = new Map();
  let last = 0;
  window.webtest = {
    ask: (what, params) => new Promise(answer => {
      const id = ++last;
      waiting.set(id, answer);
      webtestAsk(JSON.stringify({id, what, params}));
    }),
    answer: (id, value) => { waiting.get(id)(value); waiting.delete(id); },
  };
}
WebAssembly.instantiateStreaming(fetch(of + ".test.wasm"), go.importObject)
  .then(r => go.run(r.instance))
  .catch(e => { add(String(e)); go.exit(1); });
</script>`

// progress asks the page for what the tests printed since it last asked.
const progress = `window.testResult ? JSON.stringify({
  done: testResult.done, code: testResult.code,
  more: testResult.output.slice(testResult.read, testResult.read = testResult.output.length),
}) : ""`

func main() {
	chrome := flag.String("chrome", "", "the browser to run, or the first Chrome or Chromium on the PATH")
	serve := flag.String("serve", "", "only serve the page at this address, such as 127.0.0.1:8784")
	timeout := flag.Duration("timeout", 5*time.Minute, "how long the tests may take")
	only := flag.String("run", "", "run only driver/web's tests that match, as go test -run does")
	flag.Parse()
	if err := run(*chrome, *serve, *only, *timeout); err != nil {
		log.Fatal("webtest: ", err)
	}
}

func run(chrome, serve, only string, timeout time.Duration) error {
	dir, err := os.MkdirTemp("", "gunim-webtest")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := build(ctx, dir); err != nil {
		return err
	}
	handler := http.FileServer(http.Dir(dir))
	if serve != "" {
		fmt.Printf("open http://%s/ and http://%s/?of=text\n", serve, serve)
		return http.ListenAndServe(serve, handler)
	}
	l, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer func() { _ = l.Close() }()
	go func() { _ = http.Serve(l, handler) }()
	for _, t := range tested {
		address := "http://" + l.Addr().String() + "/?of=" + t.name
		if only != "" {
			if t.name != "web" {
				continue
			}
			address += "&run=" + url.QueryEscape(only)
		}
		err = runInChrome(ctx, chrome, dir, address)
		if ctx.Err() != nil {
			return fmt.Errorf("no result in %v", timeout)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// tested are the packages whose tests run. Of the package text's, those named for the browser do.
var tested = []struct{ name, dir string }{{"web", "./driver/web"}, {"text", "./text"}}

// build leaves the tests, Go's loader for them and the page in dir.
func build(ctx context.Context, dir string) error {
	for _, t := range tested {
		test := exec.CommandContext(ctx, "go", "test", "-c", "-o", filepath.Join(dir, t.name+".test.wasm"), t.dir)
		test.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
		test.Stdout, test.Stderr = os.Stdout, os.Stderr
		if err := test.Run(); err != nil {
			return fmt.Errorf("building the tests: %w", err)
		}
	}
	root, err := exec.CommandContext(ctx, "go", "env", "GOROOT").Output()
	if err != nil {
		return err
	}
	loader, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(root)), "lib", "wasm", "wasm_exec.js"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "wasm_exec.js"), loader, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, saveworker.Name), saveworker.Script, 0o600); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.html"), []byte(page), 0o600)
}

func findChrome() (string, error) {
	for _, name := range []string{
		"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "chrome",
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", errors.New("no Chrome found: name one with -chrome")
}

func runInChrome(ctx context.Context, chrome, dir, address string) error {
	if runtime.GOOS == "windows" {
		return errors.New("no debugging pipe to Chrome on Windows: use -serve and open the page")
	}
	if chrome == "" {
		var err error
		if chrome, err = findChrome(); err != nil {
			return err
		}
	}
	// Chrome reads commands from file descriptor 3 and answers on 4.
	commandsR, commandsW, err := os.Pipe()
	if err != nil {
		return err
	}
	answersR, answersW, err := os.Pipe()
	if err != nil {
		return err
	}
	profile, err := os.MkdirTemp(dir, "profile")
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, chrome, "--headless=new", "--remote-debugging-pipe",
		"--user-data-dir="+profile, "--no-first-run", "--no-default-browser-check",
		"--no-sandbox", "--use-angle=swiftshader", "--enable-unsafe-swiftshader", "--window-size=1280,800",
		"about:blank")
	cmd.ExtraFiles = []*os.File{commandsR, answersW}
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = commandsR.Close()
	_ = answersW.Close()
	defer func() {
		_ = commandsW.Close() // Chrome ends when this closes.
		_ = cmd.Wait()
	}()
	return runTests(&debugger{to: commandsW, from: bufio.NewReader(answersR)}, dir, address)
}

// runTests opens the page with window focus, and prints what it prints. dir is
// where it makes the files a test asks for.
func runTests(c *debugger, dir, address string) error {
	// A download the tests make goes nowhere, should one get past them.
	if err := c.call("", "Browser.setDownloadBehavior", map[string]any{"behavior": "deny"}, nil); err != nil {
		return err
	}
	var target struct{ TargetID string }
	if err := c.call("", "Target.createTarget", map[string]any{"url": "about:blank"}, &target); err != nil {
		return err
	}
	var attached struct{ SessionID string }
	if err := c.call("", "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true}, &attached); err != nil {
		return err
	}
	session := attached.SessionID
	if err := c.call(session, "Emulation.setFocusEmulationEnabled", map[string]any{"enabled": true}, nil); err != nil {
		return err
	}
	for _, step := range []struct {
		method string
		params map[string]any
	}{
		{"Runtime.enable", nil},
		{"Runtime.addBinding", map[string]any{"name": "webtestAsk"}},
		{"Page.enable", nil},
		{"Page.setInterceptFileChooserDialog", map[string]any{"enabled": true}},
		{"Page.navigate", map[string]any{"url": address}},
	} {
		if err := c.call(session, step.method, step.params, nil); err != nil {
			return err
		}
	}
	h := &helper{c: c, session: session, dir: dir}
	var output strings.Builder
	for {
		var answer struct{ Result struct{ Value string } }
		if err := c.call(session, "Runtime.evaluate", map[string]any{"expression": progress}, &answer); err != nil {
			return err
		}
		var now struct {
			Done bool
			Code int
			More string
		}
		if answer.Result.Value != "" {
			if err := json.Unmarshal([]byte(answer.Result.Value), &now); err != nil {
				return err
			}
		}
		fmt.Print(now.More)
		output.WriteString(now.More)
		if err := h.handle(); err != nil {
			return err
		}
		if now.Done {
			if now.Code != 0 {
				return fmt.Errorf("the tests failed, with code %d", now.Code)
			}
			// A skipped test didn't run, and a pass here says they all did.
			if n := strings.Count(output.String(), "--- SKIP"); n > 0 {
				return fmt.Errorf("%d tests skipped", n)
			}
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// helper does what the tests ask of the runner.
type helper struct {
	c       *debugger
	session string
	dir     string
	// chosen are the files to choose in the next picker, and made counts the
	// sets made.
	chosen []string
	made   int
}

// handle answers what the page asked since it was last called.
func (h *helper) handle() error {
	for len(h.c.events) > 0 {
		e := h.c.events[0]
		h.c.events = h.c.events[1:]
		switch e.Method {
		case "Runtime.bindingCalled":
			var ask struct {
				ID     int
				What   string
				Params json.RawMessage
			}
			var p struct{ Payload string }
			if err := json.Unmarshal(e.Params, &p); err != nil {
				return err
			}
			if err := json.Unmarshal([]byte(p.Payload), &ask); err != nil {
				return err
			}
			answer, err := h.do(ask.What, ask.Params)
			if err != nil {
				answer = map[string]any{"error": err.Error()}
			}
			js, err := json.Marshal(answer)
			if err != nil {
				return err
			}
			expr := fmt.Sprintf("webtest.answer(%d, %s)", ask.ID, js)
			if err := h.c.call(h.session, "Runtime.evaluate", map[string]any{"expression": expr}, nil); err != nil {
				return err
			}
		case "Page.fileChooserOpened":
			var p struct{ BackendNodeID int }
			if err := json.Unmarshal(e.Params, &p); err != nil {
				return err
			}
			if h.chosen == nil {
				// The test cancels it itself.
				continue
			}
			files := h.chosen
			h.chosen = nil
			if err := h.c.call(h.session, "DOM.setFileInputFiles",
				map[string]any{"files": files, "backendNodeId": p.BackendNodeID}, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// maxChosen is the most bytes the files of one choice may take, so a test cannot
// fill the disk.
const maxChosen = 400 << 20

// do does one thing a test asked: a click at x, y on the page, or files to
// choose next, each of a size, beginning with its text and the rest zeros.
func (h *helper) do(what string, params json.RawMessage) (any, error) {
	switch what {
	case "click":
		var at struct{ X, Y float64 }
		if err := json.Unmarshal(params, &at); err != nil {
			return nil, err
		}
		for _, kind := range []string{"mousePressed", "mouseReleased"} {
			if err := h.c.call(h.session, "Input.dispatchMouseEvent", map[string]any{
				"type": kind, "x": at.X, "y": at.Y, "button": "left", "clickCount": 1,
			}, nil); err != nil {
				return nil, err
			}
		}
		return map[string]any{}, nil
	case "choose":
		var files []struct {
			Name string
			Size int64
			Text string
		}
		if err := json.Unmarshal(params, &files); err != nil {
			return nil, err
		}
		h.made++
		dir := filepath.Join(h.dir, fmt.Sprintf("chosen%d", h.made))
		if err := os.Mkdir(dir, 0o700); err != nil {
			return nil, err
		}
		total := int64(0)
		paths := []string{}
		for _, f := range files {
			total += f.Size
			if total > maxChosen || f.Size < int64(len(f.Text)) || filepath.Base(f.Name) != f.Name {
				return nil, fmt.Errorf("no file %q of %d bytes", f.Name, f.Size)
			}
			p := filepath.Join(dir, f.Name)
			if err := os.WriteFile(p, []byte(f.Text), 0o600); err != nil {
				return nil, err
			}
			if err := os.Truncate(p, f.Size); err != nil {
				return nil, err
			}
			paths = append(paths, p)
		}
		h.chosen = paths
		return map[string]any{}, nil
	}
	return nil, fmt.Errorf("nothing called %q", what)
}

// debugger speaks Chrome's DevTools protocol over the pipe: a JSON message
// followed by a zero byte each way. events holds what Chrome said of itself
// that the helper hears.
type debugger struct {
	to     *os.File
	from   *bufio.Reader
	id     int
	events []event
}

// event is a message Chrome sends unasked.
type event struct {
	Method string
	Params json.RawMessage
}

// call sends a command to the session, or to the browser if that is empty, and
// reads its answer into out. It passes over the events Chrome sends between.
func (c *debugger) call(session, method string, params, out any) error {
	c.id++
	message, err := json.Marshal(struct {
		ID        int    `json:"id"`
		SessionID string `json:"sessionId,omitempty"`
		Method    string `json:"method"`
		Params    any    `json:"params"`
	}{c.id, session, method, params})
	if err != nil {
		return err
	}
	if _, err := c.to.Write(append(message, 0)); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	for {
		line, err := c.from.ReadBytes(0)
		if err != nil {
			return fmt.Errorf("%s: %w", method, err)
		}
		var answer struct {
			ID     int
			Method string
			Params json.RawMessage
			Result json.RawMessage
			Error  *struct{ Message string }
		}
		if err := json.Unmarshal(line[:len(line)-1], &answer); err != nil {
			return err
		}
		if answer.Method == "Runtime.bindingCalled" || answer.Method == "Page.fileChooserOpened" {
			c.events = append(c.events, event{answer.Method, answer.Params})
		}
		if answer.ID != c.id {
			continue
		}
		if answer.Error != nil {
			return fmt.Errorf("%s: %s", method, answer.Error.Message)
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(answer.Result, out)
	}
}
