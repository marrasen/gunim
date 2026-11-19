package text

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// cacheCorpus is text of every kind a layout handles differently: both
// directions and the two mixed, Chinese, Japanese and Korean, emoji and
// their sequences, combining marks, tabs, every line break, paths, long
// words and nothing at all.
var cacheCorpus = []string{
	"",
	" ",
	fox,
	"Keep the copies on the disk for 30 days, then remove the oldest first.",
	"שלום עולם, מה שלומך היום?",
	"The word שלום means peace, and سلام too.",
	"مرحبا بالعالم 123 and back to English",
	"一二三四五六七八九十，中文的句子没有空格可以断行。",
	"日本語のテキストとEnglishが混ざっている文",
	"한국어 문장도 줄을 바꿉니다",
	"party 🎉 time 👍🏽 family 👨‍👩‍👧 flag 🏳️‍🌈 done",
	"🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉🎉",
	"cafe\u0301 and na\u0308ive and e\u0301\u0301",
	"a\tb\tc\t\td",
	"one\ntwo\r\nthree\rfour\u2028five\u2029six\u0085seven",
	"\n\nempty lines\n\n",
	`C:\Users\someone\AppData\Local\Programs\something\file.txt`,
	"/home/someone/src/project/internal/package/file_test.go",
	"https://example.com/a/rather/long/path/to/a/page?query=1&other=2",
	"Supercalifragilisticexpialidocious_is_one_word_without_any_break",
	"\u200b\u200d\ufeff\u00ad soft\u00adhyphen",
	"Ɓ and ƀ",
}

var cacheWidths = []float32{0, -5, 1, 97.5, 333}

// TestTheCacheChangesNoLayout lays out every text in the corpus at many
// sizes, widths and styles, twice through the cache, and compares both
// with a layout made without it.
func TestTheCacheChangesNoLayout(t *testing.T) {
	cache.reset(cacheBudget)
	faces := map[string]*Face{"Go": Default(), "Go Mono": GoMono(false, false), "Inter": Inter(true)}
	styles := []Style{
		{Size: 15},
		{Size: 9.5, LineHeight: 1.4, Align: AlignCenter},
		{Size: 22, Align: AlignEnd, MaxLines: 1},
		{Size: 13.25, MaxLines: 2},
	}
	for name, f := range faces {
		for _, s := range cacheCorpus {
			for _, st := range styles {
				for _, w := range cacheWidths {
					want, _ := f.layoutUncached(s, st, w)
					for pass := range 2 {
						if got := f.Layout(s, st, w); !reflect.DeepEqual(got, want) {
							t.Fatalf("%s, %q, %+v at %v px, pass %d: the cache's layout differs from a fresh one", name, s, st, w, pass)
						}
					}
				}
				want, _ := f.shapeUncached(s, st.Size)
				for pass := range 2 {
					if got := f.Shape(s, st.Size); !reflect.DeepEqual(got, want) {
						t.Fatalf("%s, %q at %v px, pass %d: the cache's run differs from a fresh one", name, s, st.Size, pass)
					}
				}
			}
		}
	}
	if cache.hits == 0 {
		t.Fatal("no layout came from the cache")
	}
}

func TestSpansAreTheSameWithTheCache(t *testing.T) {
	spans := []Span{
		{Text: "Keep the copies for ", Face: Default(), Size: 15},
		{Text: "30 days", Face: GoSans(true, false), Size: 15},
		{Box: 12, Face: Default(), Size: 15},
		{Text: ", then שלום remove the oldest first when the disk is nearly full.", Face: Default(), Size: 11},
	}
	for _, w := range cacheWidths {
		cache.reset(cacheBudget)
		want := LayoutSpans(spans, Style{}, w)
		if got := LayoutSpans(spans, Style{}, w); !reflect.DeepEqual(got, want) {
			t.Fatalf("at %v px the spans laid out from the cache differ", w)
		}
		if cache.hits == 0 {
			t.Fatalf("at %v px no span came from the cache", w)
		}
	}
}

func TestChangingAnAnswerLeavesTheCacheAlone(t *testing.T) {
	f := Default()
	want, _ := f.layoutUncached(fox, Style{Size: 15}, 100)
	p := f.Layout(fox, Style{Size: 15}, 100)
	p.Lines[0].At.X = 99
	p.Lines[0].Run.Glyphs[0].ID = 12345
	p.Lines[0].Run.carets[0] = 99
	p.Lines[0].Run.stops[0].x = 99
	if got := f.Layout(fox, Style{Size: 15}, 100); !reflect.DeepEqual(got, want) {
		t.Fatal("changing a paragraph changed the next one the cache gave")
	}
	r := f.Shape(fox, 15)
	r.Glyphs[0].ID = 12345
	if got := f.Shape(fox, 15); got.Glyphs[0].ID == 12345 {
		t.Fatal("changing a run changed the next one the cache gave")
	}
}

// TestNewFallbacksLayOutAgain checks that the cache never gives a layout
// made before a face's fallbacks or the system fonts changed.
func TestNewFallbacksLayOutAgain(t *testing.T) {
	UseSystemFonts(false)
	defer UseSystemFonts(true)
	f := bare(t)
	inter := Inter(false)
	faceOfB := func() uint32 {
		p := f.Layout("Ɓ", Style{Size: 16}, 0)
		return p.Lines[0].Run.Glyphs[0].Face
	}
	if got := faceOfB(); got != f.id {
		t.Fatalf("Ɓ came from face %d, want the bare face %d", got, f.id)
	}
	f.Fallback(inter)
	if got := faceOfB(); got != inter.id {
		t.Fatalf("after Fallback, Ɓ came from face %d, want Inter, %d", got, inter.id)
	}
	if got := f.Shape("Ɓ", 16).Glyphs[0].Face; got != inter.id {
		t.Fatalf("after Fallback, Shape put Ɓ in face %d, want Inter, %d", got, inter.id)
	}
	f.Fallback()
	if got := faceOfB(); got != f.id {
		t.Fatalf("with the fallbacks gone, Ɓ came from face %d, want the bare face %d", got, f.id)
	}
}

func TestTheCacheStaysWithinItsBudget(t *testing.T) {
	const budget = 64 << 10
	defer cache.reset(cacheBudget)
	cache.reset(budget)
	f := Default()
	f.Layout("first", Style{Size: 13}, 0)
	for i := range 500 {
		f.Layout(fmt.Sprintf("line %d of a long log that fills the cache", i), Style{Size: 13}, 200)
	}
	cache.mu.Lock()
	cost, n := cache.cost, len(cache.entries)
	cache.mu.Unlock()
	if cost > budget {
		t.Fatalf("the cache holds %d bytes, past its budget of %d", cost, budget)
	}
	if n == 0 || n >= 500 {
		t.Fatalf("the cache holds %d entries after 501 layouts, want some but not all", n)
	}
	// The newest is still there, and the oldest went.
	if cache.get(layoutKey(f, "line 499 of a long log that fills the cache", Style{Size: 13}, 200)) == nil {
		t.Fatal("the newest layout is gone")
	}
	if cache.get(layoutKey(f, "first", Style{Size: 13}, 0)) != nil {
		t.Fatal("the oldest layout is still there")
	}

	// A layout too big for the cache is made each time and left out.
	cache.reset(budget)
	f.Layout(strings.Repeat(fox+" ", 20), Style{Size: 13}, 300)
	if len(cache.entries) != 0 {
		t.Fatal("a layout too big for the cache went in")
	}
}

// TestTheCacheIsSafeAcrossGoroutines lays out the corpus from many
// goroutines at once, while another changes a face's fallbacks, which
// starts a new generation. Run it with -race.
func TestTheCacheIsSafeAcrossGoroutines(t *testing.T) {
	cache.reset(cacheBudget)
	f := Default()
	type job struct {
		s     string
		w     float32
		want  Paragraph
		shape Run
	}
	jobs := make([]job, 0, 4*len(cacheCorpus))
	for _, s := range cacheCorpus {
		for _, w := range cacheWidths[:4] {
			p, _ := f.layoutUncached(s, Style{Size: 14}, w)
			r, _ := f.shapeUncached(s, 14)
			jobs = append(jobs, job{s, w, p, r})
		}
	}
	other := bare(t)
	stop := make(chan struct{})
	var churn sync.WaitGroup
	churn.Go(func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Millisecond):
				other.Fallback(Inter(false))
			}
		}
	})
	var wg sync.WaitGroup
	errs := make(chan string, 4)
	for g := range 4 {
		wg.Go(func() {
			for round := range 5 {
				for i := range jobs {
					j := jobs[(i+g*7+round)%len(jobs)]
					p := f.Layout(j.s, Style{Size: 14}, j.w)
					if !reflect.DeepEqual(p, j.want) {
						errs <- fmt.Sprintf("%q at %v px differs", j.s, j.w)
						return
					}
					if len(p.Lines) > 0 && len(p.Lines[0].Run.Glyphs) > 0 {
						// A caller may change what it got.
						p.Lines[0].Run.Glyphs[0].ID++
					}
					if r := f.Shape(j.s, 14); !reflect.DeepEqual(r, j.shape) {
						errs <- fmt.Sprintf("%q shaped differs", j.s)
						return
					}
				}
			}
		})
	}
	wg.Wait()
	close(stop)
	churn.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}
