package main

import (
	"slices"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/paint"
)

// The vocabulary the two halves share.
type (
	// Chat is the state the chat view renders: the projects, the current project's conversations, and the open
	// conversation's timeline and message box.
	Chat struct {
		Projects      []Project
		Project       int
		Conversations []Conversation
		// Current is the open conversation's ID, and Title its name.
		Current string
		Title   string
		Items   []Item
		Link    Link
		// Replying is the message the next one replies to, and Editing the ID of the message being edited.
		Replying Quote
		Editing  string
		Draft    Draft
		// Pending are the pictures waiting to go with the next message, and Images every picture the view shows,
		// by ID.
		Pending []Picture
		Images  map[string]*paint.Image
	}

	// Picture is a picture in a message, or waiting to go with one: its ID and its size in pixels.
	Picture struct {
		ID   string
		W, H int
	}

	// Project is a project in the rail.
	Project struct {
		Name, Short string
		Unread      int
	}

	// Conversation is a conversation in the sidebar.
	Conversation struct {
		ID, Name string
		Direct   bool
		Unread   int
	}

	// Item is a row of the timeline: a day's heading when Day is set, and a message otherwise.
	Item struct {
		Key string
		Day string
		Message
	}

	// Message is a message as the timeline shows it.
	Message struct {
		ID, Author string
		Mine       bool
		At         time.Time
		Body       string
		State      State
		Edited     bool
		Withdrawn  bool
		// Continued says the message follows one by the same author closely enough to share its heading.
		Continued bool
		Reply     Quote
		Pictures  []Picture
	}

	// Quote is the message a reply cites.
	Quote struct {
		ID, Author, Text string
		// Gone says the quoted message was withdrawn.
		Gone bool
	}

	// Draft is text the application puts in the message box. Seq changes each time, so the view sets the text
	// only when the application means to.
	Draft struct {
		Text string
		Seq  int
	}

	// Typing is the patch that says who is typing in the open conversation. Who is empty when nobody is.
	Typing struct{ Who string }
)

// same reports whether two rows show the same.
func same(a, b Item) bool {
	return a.Key == b.Key && a.Day == b.Day && a.Message.equal(b.Message) && slices.Equal(a.Pictures, b.Pictures)
}

// equal compares two messages but for their pictures, which same compares.
func (m Message) equal(o Message) bool {
	type plain struct {
		ID, Author                   string
		Mine                         bool
		At                           time.Time
		Body                         string
		State                        State
		Edited, Withdrawn, Continued bool
		Reply                        Quote
	}
	flat := func(m Message) plain {
		return plain{m.ID, m.Author, m.Mine, m.At, m.Body, m.State, m.Edited, m.Withdrawn, m.Continued, m.Reply}
	}
	return flat(m) == flat(o)
}

// State is where a message of the user's own is on its way to the server.
type State uint8

const (
	Sent State = iota
	Pending
	Failed
)

// Link is the state of the connection to the server.
type Link uint8

const (
	Online Link = iota
	Offline
	Reconnecting
)

// Intents.
type (
	ProjectChosen      struct{ Index int }
	ConversationChosen struct{ ID string }
	Submitted          struct{ Text string }
	// Drafted travels as the user types in the message box.
	Drafted struct{ Text string }
	// ImagePasted travels when the user pastes a picture, as PNG, and PictureRemoved when they take one waiting
	// off the next message.
	ImagePasted    struct{ PNG []byte }
	PictureRemoved struct{ ID string }
	ReplyAsked     struct{ ID string }
	EditAsked      struct{ ID string }
	WithdrawAsked  struct{ ID string }
	RetryAsked     struct{ ID string }
	// Cancelled travels when the user drops a reply or an edit.
	Cancelled    struct{}
	LinkToggled  struct{}
	ThemeToggled struct{}
)

func init() {
	gunim.RegisterType[Chat]("chat")
	gunim.RegisterType[Typing]("chat.typing")
	gunim.RegisterType[ProjectChosen]("chat.project")
	gunim.RegisterType[ConversationChosen]("chat.conversation")
	gunim.RegisterType[Submitted]("chat.submit")
	gunim.RegisterType[Drafted]("chat.draft")
	gunim.RegisterType[ImagePasted]("chat.image")
	gunim.RegisterType[PictureRemoved]("chat.image.remove")
	gunim.RegisterType[ReplyAsked]("chat.reply")
	gunim.RegisterType[EditAsked]("chat.edit")
	gunim.RegisterType[WithdrawAsked]("chat.withdraw")
	gunim.RegisterType[RetryAsked]("chat.retry")
	gunim.RegisterType[Cancelled]("chat.cancel")
	gunim.RegisterType[LinkToggled]("chat.link")
	gunim.RegisterType[ThemeToggled]("chat.theme")
}
