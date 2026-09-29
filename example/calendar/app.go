package main

import (
	"context"
	"log"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/calendar"
)

// series is an event, which may happen more than once: its first time, how long it lasts, and how it repeats.
// Times it happens that the user moved or took away are kept by the day they would have been.
type series struct {
	id                     string
	title, location, notes string
	cal                    string
	start                  time.Time
	length                 time.Duration
	allDay                 bool
	repeat                 Repeat
	moved                  map[string]span
	gone                   map[string]bool
	// from is who invited the user, and answer what they said; fixed says it cannot be moved, as a holiday.
	from   string
	answer Answer
	fixed  bool
}

// span is a start and an end.
type span struct{ start, end time.Time }

// app is the application half: every event, and what the window shows.
type app struct {
	ctx context.Context
	c   gunim.Client
	rng *rand.Rand
	in  chan gunim.Intent
	// later carries work the timers hand back to the loop in serve.
	later chan func()

	cals   []Calendar
	series []*series
	nextID int

	view  View
	day   time.Time
	light bool
	// editing is the editor's draft, while it is open, and deleting the event a delete dialog asks about.
	editing  *Draft
	deleting string
}

func newApp(ctx context.Context, c gunim.Client, seed uint64, now time.Time) *app {
	a := &app{ctx: ctx, c: c, rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
		later: make(chan func(), 16), in: make(chan gunim.Intent, 16), view: WeekView, day: calendar.Day(now)}
	a.seed(now)
	return a
}

// dayKey names a day, for the times of a series the user changed.
func dayKey(t time.Time) string { return t.Format(time.DateOnly) }

// occurrenceID names the time a series happens on the day it would be.
func occurrenceID(s *series, day time.Time) string {
	if s.repeat == Never {
		return s.id
	}
	return s.id + "@" + dayKey(day)
}

// find returns the series and the day of the time it happens that id names.
func (a *app) find(id string) (*series, time.Time, bool) {
	sid, day, _ := strings.Cut(id, "@")
	for _, s := range a.series {
		if s.id != sid {
			continue
		}
		if day == "" {
			return s, calendar.Day(s.start), true
		}
		t, err := time.ParseInLocation(time.DateOnly, day, s.start.Location())
		if err != nil {
			return nil, time.Time{}, false
		}
		return s, t, true
	}
	return nil, time.Time{}, false
}

// next returns the day after day that s happens on.
func (s *series) next(day time.Time) time.Time {
	switch s.repeat {
	case Daily:
		return calendar.AddDays(day, 1)
	case Weekdays:
		d := calendar.AddDays(day, 1)
		for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			d = calendar.AddDays(d, 1)
		}
		return d
	case Weekly:
		return calendar.AddDays(day, 7)
	case Monthly:
		return day.AddDate(0, 1, 0)
	}
	return time.Time{}
}

// times returns each time s happens that touches the days from from to to, with the day it would be.
func (s *series) times(from, to time.Time) []struct {
	day        time.Time
	start, end time.Time
} {
	type occ = struct {
		day        time.Time
		start, end time.Time
	}
	var out []occ
	offset := s.start.Sub(calendar.Day(s.start))
	add := func(day time.Time) {
		key := dayKey(day)
		if s.gone[key] {
			return
		}
		st := day.Add(offset)
		if s.allDay {
			st = day
		}
		en := st.Add(s.length)
		if m, ok := s.moved[key]; ok {
			st, en = m.start, m.end
		}
		if st.Before(to) && en.After(from) {
			out = append(out, occ{day, st, en})
		}
	}
	day := calendar.Day(s.start)
	if s.repeat == Never {
		add(day)
		return out
	}
	// A moved time may land in the range from a day outside it, so look a week either side.
	for early := calendar.AddDays(from, -7); day.Before(to.AddDate(0, 0, 7)); day = s.next(day) {
		if !day.Before(early) {
			add(day)
		}
	}
	return out
}

// shownDays returns the days the view shows.
func (a *app) shownDays() (time.Time, time.Time) {
	switch a.view {
	case DayView:
		return a.day, calendar.AddDays(a.day, 1)
	case WeekView:
		f := calendar.WeekStart(a.day, time.Monday)
		return f, calendar.AddDays(f, 7)
	}
	f := calendar.WeekStart(calendar.MonthStart(a.day), time.Monday)
	return f, calendar.AddDays(f, 42)
}

// title names the days the view shows.
func (a *app) title() string {
	switch a.view {
	case DayView:
		return a.day.Format("Monday 2 January 2006")
	case WeekView:
		from, to := a.shownDays()
		to = calendar.AddDays(to, -1)
		_, week := from.ISOWeek()
		w := "Week " + strconv.Itoa(week) + " · "
		if from.Month() == to.Month() {
			return w + from.Format("2") + "–" + to.Format("2 January 2006")
		}
		if from.Year() == to.Year() {
			return w + from.Format("2 Jan") + " – " + to.Format("2 Jan 2006")
		}
		return w + from.Format("2 Jan 2006") + " – " + to.Format("2 Jan 2006")
	}
	return a.day.Format("January 2006")
}

// calendarOf returns the calendar with id.
func (a *app) calendarOf(id string) (Calendar, bool) {
	i := slices.IndexFunc(a.cals, func(c Calendar) bool { return c.ID == id })
	if i < 0 {
		return Calendar{}, false
	}
	return a.cals[i], true
}

// state returns what the window shows.
func (a *app) state() Cal {
	s := Cal{View: a.view, Day: a.day, Title: a.title(), Calendars: a.cals, Details: map[string]Details{}}
	from, to := a.shownDays()
	for _, sr := range a.series {
		if sr.answer == NoAnswer {
			s.Invites++
		}
		c, ok := a.calendarOf(sr.cal)
		if !ok || !c.Shown || sr.answer == NotGoing {
			continue
		}
		for _, o := range sr.times(from, to) {
			id := occurrenceID(sr, o.day)
			s.Events = append(s.Events, calendar.Event{ID: id, Title: sr.title, Location: sr.location,
				Start: o.start, End: o.end, AllDay: sr.allDay, Color: c.Color, Faint: sr.answer == NoAnswer,
				Fixed: sr.fixed})
			s.Details[id] = Details{Calendar: c.Name, Repeat: sr.repeat, Notes: sr.notes, From: sr.from,
				Answer: sr.answer}
		}
	}
	return s
}

// publish sends the window the state as it is now.
func (a *app) publish() {
	if err := a.c.Update("cal", a.state()); err != nil {
		log.Print(err)
	}
}

// patch sends the window a patch.
func (a *app) patch(v any) {
	if err := a.c.Patch("cal", v); err != nil {
		log.Print(err)
	}
}

// after runs fn on the loop in serve after d.
func (a *app) after(d time.Duration, fn func()) {
	time.AfterFunc(d, func() {
		select {
		case a.later <- fn:
		case <-a.ctx.Done():
		}
	})
}

// serve runs the application until the context ends or the window closes.
func (a *app) serve() error {
	if err := a.c.Mount(gunim.Root, "cal", "cal", a.state()); err != nil {
		return err
	}
	go func() {
		for ev := range a.c.Intents() {
			select {
			case a.in <- ev.Intent:
			case <-a.ctx.Done():
				return
			}
		}
		close(a.in)
	}()
	a.after(a.between(20*time.Second, 40*time.Second), a.invite)
	for {
		select {
		case <-a.ctx.Done():
			return nil
		case fn := <-a.later:
			fn()
		case v, ok := <-a.in:
			if !ok {
				return a.c.Err()
			}
			a.handle(v)
		}
	}
}

// between returns a random duration from lo to hi.
func (a *app) between(lo, hi time.Duration) time.Duration {
	return lo + time.Duration(a.rng.Int64N(int64(hi-lo)))
}

// handle acts on an intent from the window.
func (a *app) handle(v gunim.Intent) {
	switch v := v.(type) {
	case ViewChosen:
		a.view = v.View
	case Stepped:
		switch a.view {
		case DayView:
			a.day = calendar.AddDays(a.day, v.By)
		case WeekView:
			a.day = calendar.AddDays(a.day, 7*v.By)
		case MonthView:
			a.day = calendar.MonthStart(a.day).AddDate(0, v.By, 0)
		}
	case TodayAsked:
		a.day = calendar.Day(time.Now())
	case DayPicked:
		a.day = calendar.Day(v.Day)
	case DayOpened:
		a.day, a.view = calendar.Day(v.Day), DayView
	case CalendarToggled:
		for i := range a.cals {
			if a.cals[i].ID == v.ID {
				a.cals[i].Shown = !a.cals[i].Shown
			}
		}
	case EventDrawn:
		a.edit(Draft{Start: v.Start, End: v.End, AllDay: v.AllDay, Calendar: a.cals[0].ID})
	case NewAsked:
		// The next hour today, or nine in the morning when that is outside the working day, or not today.
		h := time.Now().Hour() + 1
		if !calendar.SameDay(a.day, time.Now()) || h < 8 || h > 18 {
			h = 9
		}
		start := a.day.Add(time.Duration(h) * time.Hour)
		a.edit(Draft{Start: start, End: start.Add(time.Hour), Calendar: a.cals[0].ID})
	case EventChanged:
		a.change(v.ID, v.Start, v.End)
	case EditAsked:
		if d, ok := a.draftOf(v.ID); ok {
			a.edit(d)
		}
	case Saved:
		a.save(v.Draft)
		a.closeEditor()
	case EditorClosed:
		a.closeEditor()
	case DeleteAsked:
		a.askDelete(v.ID)
	case DeleteAnswered:
		a.delete(v)
	case Answered:
		if s, _, ok := a.find(v.ID); ok && s.answer != NotInvited {
			s.answer = v.Answer
		}
	case SearchAsked:
		a.patch(Found{Items: a.search(v.Query, time.Now())})
		return
	case EventShown:
		if !a.show(v.ID) {
			return
		}
		a.publish()
		a.patch(Reveal{ID: v.ID})
		return
	case ThemeToggled:
		a.light = !a.light
		if err := a.c.SetTheme(map[bool]string{false: "dark", true: "light"}[a.light]); err != nil {
			log.Print(err)
		}
		return
	default:
		return
	}
	a.publish()
}

// change moves the time id names to start and end. For a series that repeats only that time moves.
func (a *app) change(id string, start, end time.Time) {
	s, day, ok := a.find(id)
	if !ok || s.fixed {
		return
	}
	if s.repeat == Never {
		s.start, s.length = start, end.Sub(start)
		if s.allDay {
			s.start = calendar.Day(start)
		}
		return
	}
	if s.moved == nil {
		s.moved = map[string]span{}
	}
	s.moved[dayKey(day)] = span{start, end}
}

// draftOf returns the editor's draft for the time id names.
func (a *app) draftOf(id string) (Draft, bool) {
	s, day, ok := a.find(id)
	if !ok || s.fixed {
		return Draft{}, false
	}
	occ := s.times(day, calendar.AddDays(day, 1))
	st, en := s.start, s.start.Add(s.length)
	for _, o := range occ {
		if o.day.Equal(day) {
			st, en = o.start, o.end
		}
	}
	return Draft{ID: id, Title: s.title, Location: s.location, Notes: s.notes, Calendar: s.cal, Start: st, End: en,
		AllDay: s.allDay, Repeat: s.repeat, Series: s.repeat != Never}, true
}

// edit opens the editor on d.
func (a *app) edit(d Draft) {
	if a.editing != nil {
		return
	}
	d.Calendars = a.cals
	a.editing = &d
	if err := a.c.Mount(gunim.Root, "editor", "editor", d); err != nil {
		log.Print(err)
		a.editing = nil
	}
}

// closeEditor takes the editor away.
func (a *app) closeEditor() {
	if a.editing == nil {
		return
	}
	a.editing = nil
	if err := a.c.Unmount("editor"); err != nil {
		log.Print(err)
	}
}

// save keeps what the editor holds: a new event, or a change to one, which for a series goes to every time it
// happens.
func (a *app) save(d Draft) {
	if d.AllDay {
		d.Start, d.End = calendar.Day(d.Start), calendar.AddDays(calendar.Day(d.End), 1)
		if !d.End.After(d.Start) {
			d.End = calendar.AddDays(d.Start, 1)
		}
	}
	if d.ID == "" {
		a.nextID++
		a.series = append(a.series, &series{id: "e" + strconv.Itoa(a.nextID), title: d.Title, location: d.Location,
			notes: d.Notes, cal: d.Calendar, start: d.Start, length: d.End.Sub(d.Start), allDay: d.AllDay,
			repeat: d.Repeat})
		return
	}
	s, day, ok := a.find(d.ID)
	if !ok {
		return
	}
	s.title, s.location, s.notes, s.cal, s.allDay = d.Title, d.Location, d.Notes, d.Calendar, d.AllDay
	s.length = d.End.Sub(d.Start)
	if s.repeat == Never {
		s.start = d.Start
	} else {
		// The whole series shifts by as much as this time moved, and this time loses its own place.
		orig := day.Add(s.start.Sub(calendar.Day(s.start)))
		if s.allDay {
			orig = day
		}
		s.start = s.start.Add(d.Start.Sub(orig))
		delete(s.moved, dayKey(day))
	}
	s.repeat = d.Repeat
}

// askDelete asks whether to delete the time id names.
func (a *app) askDelete(id string) {
	s, _, ok := a.find(id)
	if !ok || s.fixed || a.deleting != "" {
		return
	}
	a.deleting = id
	if err := a.c.Mount(gunim.Root, "delete", "delete", DeleteAsk{ID: id, Title: s.title,
		Series: s.repeat != Never}); err != nil {
		log.Print(err)
		a.deleting = ""
	}
}

// delete acts on the answer to a delete dialog.
func (a *app) delete(v DeleteAnswered) {
	if a.deleting == "" {
		return
	}
	a.deleting = ""
	if err := a.c.Unmount("delete"); err != nil {
		log.Print(err)
	}
	if !v.OK {
		return
	}
	s, day, ok := a.find(v.ID)
	if !ok {
		return
	}
	if s.repeat == Never || v.All {
		a.series = slices.DeleteFunc(a.series, func(o *series) bool { return o == s })
		return
	}
	if s.gone == nil {
		s.gone = map[string]bool{}
	}
	s.gone[dayKey(day)] = true
}
