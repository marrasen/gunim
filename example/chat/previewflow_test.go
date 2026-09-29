package main

import "testing"

func TestALinkSentGetsItsCard(t *testing.T) {
	s := site(t, `<meta property="og:title" content="Release 5.3"><meta property="og:image" content="/pic.png">`)
	h := newHarness(t)
	h.a.web = s.Client()
	h.typeAndSend("See " + s.URL + "/page for the notes.")
	body := "See " + s.URL + "/page for the notes."
	h.until("the card arrives", func() bool {
		it, ok := h.item(body)
		return ok && it.Preview.Title == "Release 5.3"
	})
	it, _ := h.item(body)
	if it.Preview.URL != s.URL+"/page" || it.Preview.Picture == "" || h.a.images[it.Preview.Picture] == nil {
		t.Fatalf("preview %+v, want the page's address and picture", it.Preview)
	}
}

func TestNoCardIsFetchedOffline(t *testing.T) {
	s := site(t, `<title>Anything</title>`)
	h := newHarness(t)
	h.a.web = s.Client()
	h.a.handle(LinkToggled{})
	h.typeAndSend("Offline " + s.URL + "/page")
	h.frames(30)
	if it, _ := h.item("Offline " + s.URL + "/page"); it.Preview.URL != "" {
		t.Fatalf("a card came offline: %+v", it.Preview)
	}
}
