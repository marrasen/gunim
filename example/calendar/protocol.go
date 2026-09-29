package main

import (
	"image/color"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/calendar"
)

// The state the application publishes, and the patches.
type (
	// Cal is what the main window shows: the view and the day it is about, its title, the calendars, and the events
	// in the days it shows, with more about each for its card.
	Cal struct {
		View      View
		Day       time.Time
		Title     string
		Calendars []Calendar
		Events    []calendar.Event
		Details   map[string]Details
		// Invites is how many invitations wait for an answer, and Busy says a dialog is open.
		Invites int
		Busy    bool
		// LastCalendar is the calendar the user last put an event in, for the next to go in too.
		LastCalendar string
	}

	// Calendar is one of the user's calendars, such as their work's: its colour, and whether its events show.
	Calendar struct {
		ID, Name string
		Color    color.NRGBA
		Shown    bool
	}

	// Details is more about an event, for its card: the calendar it is in, how it repeats, notes, who invited the
	// user, and their answer.
	Details struct {
		Calendar string
		Repeat   Repeat
		Notes    string
		From     string
		Answer   Answer
	}

	// Draft is an event as the editor holds it. A new event has no ID. Series says the event repeats, so a change
	// goes to every time it happens.
	Draft struct {
		ID                     string
		Title, Location, Notes string
		Calendar               string
		Start, End             time.Time
		AllDay                 bool
		Repeat                 Repeat
		Series                 bool
		// Calendars are the calendars the editor offers, by ID and name.
		Calendars []Calendar
	}

	// Notice is a patch that shows a short notice about an event, such as an invitation that came.
	Notice struct {
		Title, Body, ID string
		// Undo offers to undo what the notice tells of, in place of showing the event.
		Undo bool
	}

	// Found is a patch that answers a search with the events found.
	Found struct{ Items []FoundEvent }

	// FoundEvent is an event a search found: when it is, and its calendar's colour.
	FoundEvent struct {
		ID, Title, When string
		Color           color.NRGBA
	}

	// Reveal is a patch that brings the event ID into sight and opens its card.
	Reveal struct{ ID string }

	// ChangeAsk asks whether moving one time of a repeating event moves only that time or every time.
	ChangeAsk struct {
		ID, Title string
	}

	// DeleteAsk asks whether to delete an event, and for one that repeats, whether only this time or every time.
	DeleteAsk struct {
		ID, Title string
		Series    bool
	}
)

// View is how the window shows time: a day, a week, or a month.
type View uint8

// The views.
const (
	DayView View = iota
	WeekView
	MonthView
)

// Repeat is how an event repeats.
type Repeat uint8

// The ways an event repeats.
const (
	Never Repeat = iota
	Daily
	Weekdays
	Weekly
	Monthly
)

// repeatNames name the ways an event repeats, in order.
var repeatNames = []string{"Does not repeat", "Every day", "Every weekday", "Every week", "Every month"}

// Answer is what the user said to an invitation.
type Answer uint8

// The answers. Not invited is an event of the user's own.
const (
	NotInvited Answer = iota
	NoAnswer
	Going
	NotGoing
)

// Intents.
type (
	ViewChosen struct{ View View }
	// Stepped moves the view by that many days, weeks or months, and TodayAsked brings it to today.
	Stepped    struct{ By int }
	TodayAsked struct{}
	// DayPicked shows the day in the view; DayOpened shows it alone.
	DayPicked       struct{ Day time.Time }
	DayOpened       struct{ Day time.Time }
	CalendarToggled struct{ ID string }
	// EventDrawn travels when the user draws out a new event, NewAsked when they ask for one by the button.
	EventDrawn struct {
		Start, End time.Time
		AllDay     bool
	}
	NewAsked struct{}
	// EventChanged travels when the user moves an event or changes its length.
	EventChanged struct {
		ID         string
		Start, End time.Time
	}
	EditAsked   struct{ ID string }
	DeleteAsked struct{ ID string }
	// Answered travels when the user answers an invitation.
	Answered struct {
		ID     string
		Answer Answer
	}
	// Saved travels when the user saves the editor, and EditorClosed when they close it without.
	Saved        struct{ Draft Draft }
	EditorClosed struct{}
	// DeleteAnswered answers a DeleteAsk: OK deletes, and All deletes every time a repeating event happens.
	DeleteAnswered struct {
		ID      string
		OK, All bool
	}
	ThemeToggled struct{}
	// SearchAsked travels as the user types in the search, and EventShown when they pick an event to see, from the
	// search or a notice.
	SearchAsked struct{ Query string }
	EventShown  struct{ ID string }
	// UndoAsked undoes the last change, InvitesAsked shows the next invitation waiting for an answer, and
	// MoreAsked opens the full editor on what a quick one holds.
	UndoAsked struct{}
	// DuplicateAsked opens the editor on a copy of an event, and CalendarSet moves an event to another calendar.
	DuplicateAsked struct{ ID string }
	CalendarSet    struct{ ID, Calendar string }
	InvitesAsked   struct{}
	MoreAsked      struct{ Draft Draft }
	// ChangeAnswered answers a ChangeAsk: OK keeps the move, and All moves every time.
	ChangeAnswered struct {
		ID      string
		OK, All bool
	}
)

func init() {
	gunim.RegisterType[Cal]("cal")
	gunim.RegisterType[Draft]("cal.draft")
	gunim.RegisterType[DeleteAsk]("cal.delete.ask")
	gunim.RegisterType[ViewChosen]("cal.view")
	gunim.RegisterType[Stepped]("cal.step")
	gunim.RegisterType[TodayAsked]("cal.today")
	gunim.RegisterType[DayPicked]("cal.day")
	gunim.RegisterType[DayOpened]("cal.day.open")
	gunim.RegisterType[CalendarToggled]("cal.calendar")
	gunim.RegisterType[EventDrawn]("cal.draw")
	gunim.RegisterType[NewAsked]("cal.new")
	gunim.RegisterType[EventChanged]("cal.change")
	gunim.RegisterType[EditAsked]("cal.edit")
	gunim.RegisterType[DeleteAsked]("cal.delete")
	gunim.RegisterType[Answered]("cal.answer")
	gunim.RegisterType[Saved]("cal.save")
	gunim.RegisterType[EditorClosed]("cal.editor.close")
	gunim.RegisterType[DeleteAnswered]("cal.delete.answer")
	gunim.RegisterType[ThemeToggled]("cal.theme")
	gunim.RegisterType[Notice]("cal.notice")
	gunim.RegisterType[Found]("cal.found")
	gunim.RegisterType[Reveal]("cal.reveal")
	gunim.RegisterType[SearchAsked]("cal.search")
	gunim.RegisterType[EventShown]("cal.show")
	gunim.RegisterType[ChangeAsk]("cal.change.ask")
	gunim.RegisterType[UndoAsked]("cal.undo")
	gunim.RegisterType[DuplicateAsked]("cal.duplicate")
	gunim.RegisterType[CalendarSet]("cal.calendar.set")
	gunim.RegisterType[InvitesAsked]("cal.invites")
	gunim.RegisterType[MoreAsked]("cal.more")
	gunim.RegisterType[ChangeAnswered]("cal.change.answer")
}
