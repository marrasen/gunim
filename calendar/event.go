// Package calendar holds the views of a calendar: a grid of days by the hour, a month, and a small month for
// picking a day, with fields for a date and a time.
//
// The views show [Event] values the application gives them, and report what the user does as intents: picking a
// day, drawing out a new event, moving one or changing its length. They keep no events of their own.
package calendar

import (
	"image/color"
	"time"
)

// Event is an event as the views show it: its ID, its title and where it is, when it starts and ends, and its
// colour. End is the first moment after it. An all-day event starts and ends at midnight.
type Event struct {
	ID       string
	Title    string
	Location string
	Start    time.Time
	End      time.Time
	AllDay   bool
	Color    color.NRGBA
	// Faint shows the event dimmed, such as one the user has not said they will go to.
	Faint bool
	// Fixed says the user cannot move the event or change its length, such as a holiday.
	Fixed bool
}

// Day returns the midnight that starts t's day, in t's location.
func Day(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// AddDays returns the midnight n days after day's, counting days on the calendar, so a day with a clock change in
// it counts as one.
func AddDays(day time.Time, n int) time.Time {
	y, m, d := day.Date()
	return time.Date(y, m, d+n, 0, 0, 0, 0, day.Location())
}

// WeekStart returns the midnight that starts the week holding t, for weeks that start on first.
func WeekStart(t time.Time, first time.Weekday) time.Time {
	back := (int(t.Weekday()) - int(first) + 7) % 7
	return AddDays(Day(t), -back)
}

// MonthStart returns the midnight that starts t's month.
func MonthStart(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
}

// SameDay reports whether a and b fall on the same day of the calendar.
func SameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// dayOffset returns how far into its day t is.
func dayOffset(t time.Time) time.Duration { return t.Sub(Day(t)) }

// covers reports whether e takes up some of the day that starts at day.
func (e Event) covers(day time.Time) bool {
	next := AddDays(day, 1)
	if e.End.Equal(e.Start) {
		return !e.Start.Before(day) && e.Start.Before(next)
	}
	return e.Start.Before(next) && e.End.After(day)
}

// long reports whether e shows in the row of whole days: an all-day event, or one that runs past a day's end.
func (e Event) long() bool {
	return e.AllDay || e.End.Sub(e.Start) >= 24*time.Hour
}
