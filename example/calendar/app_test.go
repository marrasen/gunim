package main

import (
	"context"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/calendar"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/widget"
)

// harness runs the app half against the real views in an offscreen window, on a Tuesday.
type harness struct {
	t *testing.T
	w *gunim.Window
	a *app
}

// tuesday is the day the tests are about.
var tuesday = time.Date(2026, 9, 29, 12, 0, 0, 0, time.Local)

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, w: gunimtest.New(t, geom.Sz(1280, 800), widget.NewSurface())}
	registerViews(h.w)
	gunim.RegisterPatch(h.w, "cal", func(v *calView, p probe, u *gunim.UI) { p.fn(v, u) })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h.a = newApp(ctx, h.w.Client(), 1, tuesday)
	if err := h.w.Client().Mount(gunim.Root, "cal", "cal", h.a.state()); err != nil {
		t.Fatal(err)
	}
	h.frames(10)
	return h
}

// frames draws n frames, handling the intents that come meanwhile.
func (h *harness) frames(n int) {
	for range n {
		h.w.Frame(time.Second / 60)
		for more := true; more; {
			select {
			case ev := <-h.w.Client().Intents():
				h.a.handle(ev.Intent)
			default:
				more = false
			}
		}
	}
}

// events returns the events shown with title.
func (h *harness) events(title string) []calendar.Event {
	var out []calendar.Event
	for _, e := range h.a.state().Events {
		if e.Title == title {
			out = append(out, e)
		}
	}
	return out
}

func TestAWeekdayEventShowsOnEachWorkingDayOfTheWeek(t *testing.T) {
	h := newHarness(t)
	ups := h.events("Stand-up")
	if len(ups) != 5 {
		t.Fatalf("the stand-up shows %d times this week, want five", len(ups))
	}
	for _, e := range ups {
		if wd := e.Start.Weekday(); wd == time.Saturday || wd == time.Sunday {
			t.Fatalf("the stand-up shows on a %v", wd)
		}
		if e.Start.Hour() != 9 || e.Start.Minute() != 15 || e.End.Sub(e.Start) != 15*time.Minute {
			t.Fatalf("a stand-up runs %v to %v, want 09:15 to 09:30", e.Start, e.End)
		}
	}
	h.a.handle(ViewChosen{View: MonthView})
	if n := len(h.events("Stand-up")); n < 20 {
		t.Fatalf("the month shows the stand-up %d times, want every working day of six weeks", n)
	}
}

func TestMovingOneTimeOfARepeatingEventLeavesTheRest(t *testing.T) {
	h := newHarness(t)
	ups := h.events("Stand-up")
	moved := ups[1]
	h.a.handle(EventChanged{ID: moved.ID, Start: moved.Start.Add(2 * time.Hour), End: moved.End.Add(2 * time.Hour)})
	for _, e := range h.events("Stand-up") {
		switch {
		case e.ID == moved.ID && e.Start.Hour() != 11:
			t.Fatalf("the stand-up moved starts at %v, want 11:15", e.Start)
		case e.ID != moved.ID && e.Start.Hour() != 9:
			t.Fatalf("another stand-up moved too, to %v", e.Start)
		}
	}
}

func TestDeletingOnlyThisTimeOrEveryTime(t *testing.T) {
	h := newHarness(t)
	ups := h.events("Stand-up")
	h.a.handle(DeleteAsked{ID: ups[2].ID})
	h.a.handle(DeleteAnswered{ID: ups[2].ID, OK: true})
	if n := len(h.events("Stand-up")); n != 4 {
		t.Fatalf("after deleting one time, the stand-up shows %d times, want four", n)
	}
	h.a.handle(DeleteAsked{ID: ups[0].ID})
	h.a.handle(DeleteAnswered{ID: ups[0].ID, OK: true, All: true})
	if n := len(h.events("Stand-up")); n != 0 {
		t.Fatalf("after deleting every time, the stand-up shows %d times", n)
	}
}

func TestASavedEventShowsWhereTheEditorSaid(t *testing.T) {
	h := newHarness(t)
	day := calendar.Day(tuesday)
	h.a.handle(EventDrawn{Start: day.Add(14 * time.Hour), End: day.Add(15 * time.Hour)})
	if h.a.editing == nil {
		t.Fatal("drawing out an event did not open the editor")
	}
	h.a.handle(Saved{Draft: Draft{Title: "Review", Calendar: "work", Start: day.Add(14 * time.Hour),
		End: day.Add(15*time.Hour + 30*time.Minute)}})
	if h.a.editing != nil {
		t.Fatal("saving left the editor open")
	}
	evs := h.events("Review")
	if len(evs) != 1 || evs[0].End.Sub(evs[0].Start) != 90*time.Minute {
		t.Fatalf("the saved event shows as %+v, want once, an hour and a half long", evs)
	}

	// An all-day event saved from Monday to Wednesday covers the three days.
	h.a.handle(NewAsked{})
	monday := calendar.AddDays(day, -1)
	h.a.handle(Saved{Draft: Draft{Title: "Trip", Calendar: "home", AllDay: true, Start: monday,
		End: calendar.AddDays(monday, 2)}})
	trip := h.events("Trip")
	if len(trip) != 1 || !trip[0].End.Equal(calendar.AddDays(monday, 3)) {
		t.Fatalf("the trip shows as %+v, want Monday to the end of Wednesday", trip)
	}
}

func TestEditingARepeatingEventMovesEveryTime(t *testing.T) {
	h := newHarness(t)
	up := h.events("Stand-up")[0]
	d, ok := h.a.draftOf(up.ID)
	if !ok || !d.Series {
		t.Fatalf("the stand-up's draft is %+v, want one that says it repeats", d)
	}
	d.Title = "Daily sync"
	d.Start, d.End = d.Start.Add(15*time.Minute), d.End.Add(30*time.Minute)
	h.a.handle(Saved{Draft: d})
	syncs := h.events("Daily sync")
	if len(syncs) != 5 {
		t.Fatalf("the renamed stand-up shows %d times, want five", len(syncs))
	}
	for _, e := range syncs {
		if e.Start.Hour() != 9 || e.Start.Minute() != 30 || e.End.Sub(e.Start) != 30*time.Minute {
			t.Fatalf("a sync runs %v to %v, want 09:30 to 10:00", e.Start, e.End)
		}
	}
}

func TestAnsweringAnInvitation(t *testing.T) {
	h := newHarness(t)
	ws := h.events("Workshop: faster builds")
	if len(ws) != 1 || !ws[0].Faint {
		t.Fatalf("the invitation shows as %+v, want once, faint while it waits", ws)
	}
	if h.a.state().Invites != 2 {
		t.Fatalf("%d invitations wait, want two", h.a.state().Invites)
	}
	h.a.handle(Answered{ID: ws[0].ID, Answer: Going})
	if ws := h.events("Workshop: faster builds"); len(ws) != 1 || ws[0].Faint {
		t.Fatal("going to it left it faint")
	}
	lunch := h.events("Farewell lunch")[0]
	h.a.handle(Answered{ID: lunch.ID, Answer: NotGoing})
	if len(h.events("Farewell lunch")) != 0 || h.a.state().Invites != 0 {
		t.Fatal("an invitation turned down still shows, or still waits")
	}
}

func TestHidingACalendarHidesItsEvents(t *testing.T) {
	h := newHarness(t)
	h.a.handle(CalendarToggled{ID: "team"})
	if n := len(h.events("Stand-up")); n != 0 {
		t.Fatalf("with the team calendar hidden, the stand-up shows %d times", n)
	}
	h.a.handle(CalendarToggled{ID: "team"})
	if n := len(h.events("Stand-up")); n != 5 {
		t.Fatalf("with the team calendar back, the stand-up shows %d times", n)
	}
}

func TestSteppingMovesByTheView(t *testing.T) {
	h := newHarness(t)
	h.a.handle(Stepped{By: 1})
	if want := calendar.AddDays(calendar.Day(tuesday), 7); !h.a.day.Equal(want) {
		t.Fatalf("a week on is %v, want %v", h.a.day, want)
	}
	h.a.handle(ViewChosen{View: MonthView})
	h.a.handle(Stepped{By: -1})
	if h.a.day.Month() != time.September || h.a.day.Day() != 1 {
		t.Fatalf("a month back from October is %v, want the first of September", h.a.day)
	}
	h.a.handle(DayOpened{Day: calendar.Day(tuesday)})
	if h.a.view != DayView || h.a.title() != "Tuesday 29 September 2026" {
		t.Fatalf("opening a day shows %v titled %q", h.a.view, h.a.title())
	}
	h.frames(5)
}

func TestSearchFindsTheNextTimeFirst(t *testing.T) {
	h := newHarness(t)
	found := h.a.search("stand", tuesday)
	if len(found) != 1 || found[0].Title != "Stand-up" {
		t.Fatalf("a search for stand found %+v, want the stand-up once", found)
	}
	if want := "Every weekday · next Wednesday 30 September · 09:15 – 09:30 · 15 min"; found[0].When != want {
		t.Fatalf("the stand-up is found as %q, want %q", found[0].When, want)
	}
	if found := h.a.search("congress hall", tuesday); len(found) != 1 || found[0].Title != "Conference" {
		t.Fatalf("a search by place found %+v, want the conference", found)
	}
	if found := h.a.search("", tuesday); len(found) != 0 {
		t.Fatalf("an empty search found %d events", len(found))
	}
}

func TestShowingAnEventMovesTheViewToIt(t *testing.T) {
	h := newHarness(t)
	found := h.a.search("conference", tuesday)
	h.a.handle(EventShown{ID: found[0].ID})
	h.frames(5)
	if want := calendar.AddDays(calendar.WeekStart(tuesday, time.Monday), 9); !h.a.day.Equal(want) {
		t.Fatalf("showing the conference moved the view to %v, want %v", h.a.day, want)
	}
	if len(h.events("Conference")) != 1 {
		t.Fatal("the conference does not show in the week shown")
	}
}

func TestDeletingAOneOffEventAtOnceAndUndoingIt(t *testing.T) {
	h := newHarness(t)
	dentist := h.events("Dentist")[0]
	h.a.handle(DeleteAsked{ID: dentist.ID})
	if h.a.deleting != "" {
		t.Fatal("deleting an event that happens once asked first")
	}
	if len(h.events("Dentist")) != 0 {
		t.Fatal("the dentist still shows after it was deleted")
	}
	h.a.handle(UndoAsked{})
	if len(h.events("Dentist")) != 1 {
		t.Fatal("undo did not bring the dentist back")
	}
}

func TestMovingOneTimeOfARepeatingEventAsksAndCanMoveEveryTime(t *testing.T) {
	h := newHarness(t)
	ups := h.events("Stand-up")
	later := func(e calendar.Event) EventChanged {
		return EventChanged{ID: e.ID, Start: e.Start.Add(time.Hour), End: e.End.Add(time.Hour)}
	}

	// Cancelled, nothing moves.
	h.a.handle(later(ups[1]))
	if h.a.changing == nil || !h.a.state().Busy {
		t.Fatal("moving a repeating event did not ask whether every time moves")
	}
	h.a.handle(ChangeAnswered{ID: ups[1].ID})
	for _, e := range h.events("Stand-up") {
		if e.Start.Hour() != 9 {
			t.Fatalf("after cancelling, a stand-up is at %v", e.Start)
		}
	}

	// Every time moves an hour later.
	h.a.handle(later(ups[1]))
	h.a.handle(ChangeAnswered{ID: ups[1].ID, OK: true, All: true})
	for _, e := range h.events("Stand-up") {
		if e.Start.Hour() != 10 || e.Start.Minute() != 15 {
			t.Fatalf("after moving every time, a stand-up is at %v, want 10:15", e.Start)
		}
	}

	// Undo puts them all back.
	h.a.handle(UndoAsked{})
	if e := h.events("Stand-up")[0]; e.Start.Hour() != 9 {
		t.Fatalf("after undo, the stand-up is at %v", e.Start)
	}
}

func TestMovingAnEventToAnotherCalendarAndCopyingIt(t *testing.T) {
	h := newHarness(t)
	dentist := h.events("Dentist")[0]
	h.a.handle(CalendarSet{ID: dentist.ID, Calendar: "work"})
	if d := h.a.state().Details[dentist.ID]; d.Calendar != "Work" {
		t.Fatalf("the dentist is in %q, want Work", d.Calendar)
	}
	h.a.handle(DuplicateAsked{ID: dentist.ID})
	if h.a.editing == nil || h.a.editing.ID != "" || h.a.editing.Title != "Dentist" {
		t.Fatalf("making a copy opened the editor on %+v, want a new event titled Dentist", h.a.editing)
	}
}
