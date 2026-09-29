package main

import "testing"

func TestTheFilesAreaOpensFromTheSidebarAndWalksFolders(t *testing.T) {
	h := newHarness(t)
	h.a.handle(AreaChosen{Area: "files"})
	h.frames(5)
	s := h.a.state()
	if s.Area != "files" || s.Files.Path != "" || len(s.Files.Entries) == 0 || !s.Files.Entries[0].Folder {
		t.Fatalf("the files area shows %q at %q with %d entries, want the project's top, folders first",
			s.Area, s.Files.Path, len(s.Files.Entries))
	}
	if h.v.areas.shown != 1 {
		t.Fatal("the pane still shows the conversation")
	}

	h.a.handle(FolderOpened{Path: "Design/Old"})
	h.frames(5)
	if s := h.a.state(); s.Files.Path != "Design/Old" || len(s.Files.Entries) != 1 {
		t.Fatalf("the folder open is %q with %d entries, want Design/Old with one", s.Files.Path, len(s.Files.Entries))
	}
	h.a.handle(FolderOpened{Path: "No such folder"})
	if s := h.a.state(); s.Files.Path != "Design/Old" {
		t.Fatalf("a folder that is not there moved the view to %q", s.Files.Path)
	}

	h.a.handle(ConversationChosen{ID: h.a.current.ID})
	h.frames(5)
	if s := h.a.state(); s.Area != "" || h.v.areas.shown != 0 {
		t.Fatal("choosing a conversation left the files showing")
	}
}
