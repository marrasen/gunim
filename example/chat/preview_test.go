package main

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

// site serves a page with the given head, and a 400 by 200 picture at /pic.png.
func site(t *testing.T, head string) *httptest.Server {
	t.Helper()
	var pic bytes.Buffer
	if err := png.Encode(&pic, image.NewRGBA(image.Rect(0, 0, 400, 200))); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/page", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><html><head>" + head + "</head><body><h1>Hi</h1></body></html>"))
	})
	mux.HandleFunc("/pic.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pic.Bytes())
	})
	mux.HandleFunc("/file.zip", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte("PK"))
	})
	s := httptest.NewServer(mux)
	t.Cleanup(s.Close)
	return s
}

func TestAPagesOpenGraphTagsMakeItsPreview(t *testing.T) {
	s := site(t, `<title>Plain title</title>
		<meta property="og:title" content="Release 5.3">
		<meta property="og:description" content="What is new in the app.">
		<meta property="og:site_name" content="Example">
		<meta property="og:image" content="/pic.png">`)
	p, err := fetchPage(context.Background(), s.Client(), s.URL+"/page")
	if err != nil {
		t.Fatal(err)
	}
	if p.title != "Release 5.3" || p.description != "What is new in the app." || p.site != "Example" {
		t.Fatalf("preview %+v, want the Open Graph title, description and site", p)
	}
	if p.picture == nil || p.picture.Bounds().Dx() != thumbSide || p.picture.Bounds().Dy() != thumbSide/2 {
		t.Fatalf("picture %v, want the page's picture shrunk to %d wide", p.picture, thumbSide)
	}
}

func TestAPageWithoutOpenGraphUsesItsTitle(t *testing.T) {
	s := site(t, `<title>
		The   plain   title </title><meta name="description" content="A page.">`)
	p, err := fetchPage(context.Background(), s.Client(), s.URL+"/page")
	if err != nil {
		t.Fatal(err)
	}
	if p.title != "The plain title" || p.description != "A page." || p.picture != nil {
		t.Fatalf("preview %+v, want the title, the description and no picture", p)
	}
}

func TestOnlyPagesMakePreviews(t *testing.T) {
	s := site(t, "")
	if _, err := fetchPage(context.Background(), s.Client(), s.URL+"/file.zip"); err == nil {
		t.Fatal("a zip file made a preview")
	}
	if _, err := fetchPage(context.Background(), s.Client(), s.URL+"/page"); err == nil {
		t.Fatal("a page with no title made a preview")
	}
	if _, err := fetchPage(context.Background(), s.Client(), s.URL+"/missing"); err == nil {
		t.Fatal("a missing page made a preview")
	}
}

func TestFirstLinkLeavesTheSentenceOut(t *testing.T) {
	if got := firstLink("See https://go.dev/blog/x. And more."); got != "https://go.dev/blog/x" {
		t.Fatalf("firstLink = %q", got)
	}
	if got := firstLink("no link here"); got != "" {
		t.Fatalf("firstLink = %q, want none", got)
	}
}
