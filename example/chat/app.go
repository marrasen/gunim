package main

import (
	"bytes"
	"context"
	"image/png"
	"log"
	"maps"
	"math/rand/v2"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/markdown"
	"github.com/marrasen/gunim/paint"
)

// me is the user's name.
const me = "Marcus"

// app is the application half. It holds the conversations, and stands in for the server and the colleagues on the
// other end: messages take a while to arrive, some fail, none go while offline, and colleagues type, reply, edit
// and withdraw.
type app struct {
	ctx context.Context
	c   gunim.Client
	rng *rand.Rand
	// later carries work from timers back to the loop in serve, which owns the state.
	later chan func()

	projects []*project
	project  int
	current  *conv
	link     Link
	// replying and editing are the IDs of the message the next one replies to and the one being edited.
	replying, editing string
	// text is what the message box holds, and draft what the application last put there.
	text   string
	draft  Draft
	typing string
	light  bool
	nextID int
	// failRate is the share of sends the server turns down.
	failRate float64
	// images holds every picture pasted, by ID, and pending the ones waiting to go with the next message.
	images  map[string]*paint.Image
	pending []Picture
	// web fetches link previews.
	web *http.Client
	// newFrom is the first message of the open conversation not read when it opened, and unread how many messages
	// from others it had then.
	newFrom string
	unread  int
}

type project struct {
	Name, Short string
	convs       []*conv
}

type conv struct {
	ID, Name string
	Direct   bool
	people   []string
	msgs     []*msg
	byID     map[string]*msg
	unread   int
	// readTo is the ID of the last message the user has seen, or "" for none.
	readTo  string
	project *project
}

// msg is a message as the application keeps it.
type msg struct {
	ID, Author string
	At         time.Time
	Body       string
	State      State
	Edited     bool
	Withdrawn  bool
	ReplyTo    string
	Pictures   []Picture
	// reactions are the emoji people reacted with, in the order they first came, each with who reacted.
	reactions []reaction
	// preview is the card for the first link in the text, once the sender's app has fetched it.
	preview Preview
	// poll is the message's poll, or nil.
	poll *poll
}

// poll is a question, its options, and each voter's choice.
type poll struct {
	question string
	options  []string
	votes    map[string]int
	// order is the voters in the order they first voted.
	order []string
}

// newPoll reads a poll from text written as "/poll Question | option | option", and reports false for text that
// is not one, or has fewer than two options.
func newPoll(text string) (*poll, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(text), "/poll ")
	if !ok {
		return nil, false
	}
	var parts []string
	for _, s := range strings.Split(rest, "|") {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) < 3 {
		return nil, false
	}
	return &poll{question: parts[0], options: parts[1:], votes: map[string]int{}}, true
}

// vote gives who's vote to option i, or takes it back when it was there already.
func (p *poll) vote(who string, i int) {
	if i < 0 || i >= len(p.options) {
		return
	}
	if was, ok := p.votes[who]; ok && was == i {
		delete(p.votes, who)
		p.order = slices.DeleteFunc(p.order, func(s string) bool { return s == who })
		return
	}
	if _, ok := p.votes[who]; !ok {
		p.order = append(p.order, who)
	}
	p.votes[who] = i
}

// pollOf returns m's poll as the timeline shows it.
func pollOf(m *msg) Poll {
	if m.poll == nil {
		return Poll{}
	}
	out := Poll{Question: m.poll.question, Voters: len(m.poll.votes)}
	for i, text := range m.poll.options {
		o := PollOption{Text: text}
		var who []string
		for _, v := range m.poll.order {
			if m.poll.votes[v] != i {
				continue
			}
			o.Votes++
			if v == me {
				o.Mine = true
				v = "You"
			}
			who = append(who, v)
		}
		o.Who = strings.Join(who, ", ")
		out.Options = append(out.Options, o)
	}
	return out
}

// reaction is an emoji and the people who reacted with it, in order.
type reaction struct {
	emoji string
	who   []string
}

// toggle adds who's reaction of emoji to m, or takes it back when who had it.
func (m *msg) toggle(emoji, who string) {
	for i := range m.reactions {
		r := &m.reactions[i]
		if r.emoji != emoji {
			continue
		}
		if j := slices.Index(r.who, who); j >= 0 {
			r.who = slices.Delete(r.who, j, j+1)
			if len(r.who) == 0 {
				m.reactions = slices.Delete(m.reactions, i, i+1)
			}
			return
		}
		r.who = append(r.who, who)
		return
	}
	m.reactions = append(m.reactions, reaction{emoji: emoji, who: []string{who}})
}

// reactionsOf returns m's reactions as the timeline shows them.
func reactionsOf(m *msg) []Reaction {
	var out []Reaction
	for _, r := range m.reactions {
		names := make([]string, len(r.who))
		for i, w := range r.who {
			names[i] = w
			if w == me {
				names[i] = "You"
			}
		}
		out = append(out, Reaction{Emoji: r.emoji, Count: len(r.who), Mine: slices.Contains(r.who, me), Who: strings.Join(names, ", ")})
	}
	return out
}

// pollVote returns who's choice on m's poll, and whether they chose.
func (m *msg) pollVote(who string) (int, bool) {
	if m.poll == nil {
		return 0, false
	}
	i, ok := m.poll.votes[who]
	return i, ok
}

// reactionEmoji are the emoji the pretend colleagues react with.
var reactionEmoji = []string{"\U0001F44D", "\U0001F389", "\u2764\uFE0F", "\U0001F602", "\U0001F680", "\U0001F440", "\u2705"}

func newApp(ctx context.Context, c gunim.Client, seed uint64, history int, failRate float64) *app {
	a := &app{ctx: ctx, c: c, rng: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)), later: make(chan func(), 16),
		failRate: failRate, images: map[string]*paint.Image{}, web: &http.Client{}}
	a.projects = []*project{
		a.newProject("Atlas", "AT",
			a.newConv("general", false, "Anna Berg", "Erik Lund", "Sara Nyström"),
			a.newConv("releases", false, "Anna Berg", "Erik Lund"),
			a.newConv("ci", false, "Erik Lund"),
			a.newConv("Anna Berg", true, "Anna Berg")),
		a.newProject("Beacon", "BC",
			a.newConv("general", false, "Sara Nyström", "Johan Ek"),
			a.newConv("training", false, "Johan Ek")),
		a.newProject("Cedar", "CD",
			a.newConv("general", false, "Lena Holm", "Anna Berg")),
	}
	now := time.Now().Round(0)
	for pi, p := range a.projects {
		for ci, cv := range p.convs {
			n := 30
			if pi == 0 && ci == 0 {
				n = history
			}
			a.fill(cv, n, now)
			a.leaveUnread(cv, a.rng.IntN(7))
		}
	}
	a.enter(a.projects[0].convs[0])
	return a
}

// leaveUnread marks all but the last n messages of c read, and counts those from others as unread.
func (a *app) leaveUnread(c *conv, n int) {
	c.readTo, c.unread = "", 0
	if i := len(c.msgs) - 1 - n; i >= 0 {
		c.readTo = c.msgs[i].ID
	}
	for _, m := range c.msgs[max(0, len(c.msgs)-n):] {
		if m.Author != me {
			c.unread++
		}
	}
}

// enter makes c the open conversation: the line over new messages goes over the first from someone else since the
// user last read it, and all of it is read from now on.
func (a *app) enter(c *conv) {
	a.current, a.newFrom, a.unread = c, "", c.unread
	past := c.readTo == ""
	for _, m := range c.msgs {
		if past && m.Author != me {
			a.newFrom = m.ID
			break
		}
		if m.ID == c.readTo {
			past = true
		}
	}
	if len(c.msgs) > 0 {
		c.readTo = c.msgs[len(c.msgs)-1].ID
	}
	c.unread = 0
}

func (a *app) newProject(name, short string, convs ...*conv) *project {
	p := &project{Name: name, Short: short, convs: convs}
	for _, c := range convs {
		c.project = p
	}
	return p
}

func (a *app) newConv(name string, direct bool, people ...string) *conv {
	a.nextID++
	return &conv{ID: "c" + strconv.Itoa(a.nextID), Name: name, Direct: direct, people: people, byID: map[string]*msg{}}
}

// fill gives c n messages of history, the latest a few minutes before now.
func (a *app) fill(c *conv, n int, now time.Time) {
	times := make([]time.Time, n)
	at := now.Add(-4 * time.Minute)
	for i := n - 1; i >= 0; i-- {
		times[i] = at
		at = at.Add(-time.Duration(a.rng.IntN(48)+1) * time.Minute)
	}
	for _, at := range times {
		author := me
		if a.rng.Float64() < 0.7 {
			author = a.pick(c.people)
		}
		m := a.add(c, author, a.pick(chatter), at)
		if len(c.msgs) > 3 && a.rng.Float64() < 0.12 {
			m.ReplyTo = c.msgs[len(c.msgs)-2-a.rng.IntN(2)].ID
		}
		if a.rng.Float64() < 0.04 {
			m.Edited = true
		}
		if a.rng.Float64() < 0.01 {
			m.Withdrawn = true
		}
	}
}

// add appends a message to c and returns it. Text written as "/poll Question | option | option" makes a poll.
func (a *app) add(c *conv, author, body string, at time.Time) *msg {
	a.nextID++
	m := &msg{ID: "m" + strconv.Itoa(a.nextID), Author: author, At: at, Body: body}
	if pl, ok := newPoll(body); ok {
		m.Body, m.poll = "", pl
	}
	c.msgs = append(c.msgs, m)
	c.byID[m.ID] = m
	return m
}

func (a *app) pick(s []string) string { return s[a.rng.IntN(len(s))] }

// between returns a random duration from lo to hi.
func (a *app) between(lo, hi time.Duration) time.Duration {
	return lo + time.Duration(a.rng.Int64N(int64(hi-lo)))
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
	if err := a.c.Mount(gunim.Root, "chat", "chat", a.state()); err != nil {
		return err
	}
	a.after(a.between(3*time.Second, 6*time.Second), a.colleague)
	for {
		select {
		case <-a.ctx.Done():
			return nil
		case fn := <-a.later:
			fn()
		case ev, ok := <-a.c.Intents():
			if !ok {
				return a.c.Err()
			}
			a.handle(ev.Intent)
		}
	}
}

// handle acts on an intent from the window.
func (a *app) handle(v gunim.Intent) {
	switch v := v.(type) {
	case ProjectChosen:
		if v.Index >= 0 && v.Index < len(a.projects) {
			a.project = v.Index
			a.open(a.projects[v.Index].convs[0])
		}
	case ConversationChosen:
		for _, c := range a.projects[a.project].convs {
			if c.ID == v.ID {
				a.open(c)
			}
		}
	case Drafted:
		a.text = v.Text
		return
	case ImagePasted:
		a.pastePicture(v.PNG)
	case PollVoted:
		if m, ok := a.current.byID[v.ID]; ok && m.poll != nil && !m.Withdrawn {
			m.poll.vote(me, v.Option)
		}
	case ReactionToggled:
		if m, ok := a.current.byID[v.ID]; ok && !m.Withdrawn {
			m.toggle(v.Emoji, me)
		}
	case PictureRemoved:
		a.pending = slices.DeleteFunc(a.pending, func(p Picture) bool { return p.ID == v.ID })
	case Submitted:
		a.submit(v.Text)
	case ReplyAsked:
		a.replying, a.editing = v.ID, ""
		a.setDraft(a.text)
	case EditAsked:
		if m, ok := a.current.byID[v.ID]; ok && m.Author == me && !m.Withdrawn {
			a.editing, a.replying = v.ID, ""
			a.setDraft(m.Body)
		}
	case WithdrawAsked:
		if m, ok := a.current.byID[v.ID]; ok && m.Author == me {
			m.Withdrawn = true
		}
	case RetryAsked:
		if m, ok := a.current.byID[v.ID]; ok && m.State == Failed {
			a.send(m)
		}
	case Cancelled:
		if a.editing != "" {
			a.setDraft("")
		}
		a.replying, a.editing = "", ""
	case LinkToggled:
		a.toggleLink()
	case ThemeToggled:
		a.light = !a.light
		if err := a.c.SetTheme(map[bool]string{false: "dark", true: "light"}[a.light]); err != nil {
			log.Print(err)
		}
		return
	case markdown.Link:
		a.openLink(v.URL)
		return
	case gunim.CommandFailed:
		log.Printf("command %s failed: %s", v.Command, v.Reason)
		return
	default:
		return
	}
	a.publish()
}

// openLink opens a web or mail address from a message in the system's browser or mail program. Anything else,
// such as a path to a file, stays closed: messages come from other people.
func (a *app) openLink(url string) {
	lower := strings.ToLower(url)
	if !strings.HasPrefix(lower, "https://") && !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "mailto:") {
		log.Printf("not opening %q: only web and mail addresses open", url)
		return
	}
	if err := a.c.Open(url); err != nil {
		log.Printf("opening %s: %v", url, err)
	}
}

// post runs fn on the loop in serve, as soon as the loop gets to it.
func (a *app) post(fn func()) {
	select {
	case a.later <- fn:
	case <-a.ctx.Done():
	}
}

// fetchPreview fetches the card for the first link in m, as the sender's app does, and puts it in m when it comes.
// A link with no card to make, or a fetch that fails, leaves m without one.
func (a *app) fetchPreview(m *msg) {
	link := firstLink(m.Body)
	if link == "" || a.link != Online {
		return
	}
	web := a.web
	go func() {
		p, err := fetchPage(a.ctx, web, link)
		if err != nil {
			log.Printf("preview of %s: %v", link, err)
			return
		}
		a.post(func() {
			pv := Preview{URL: p.url, Site: p.site, Title: p.title, Description: p.description}
			if p.picture != nil {
				a.nextID++
				pv.Picture = "p" + strconv.Itoa(a.nextID)
				a.images[pv.Picture] = paint.NewImage(p.picture)
			}
			m.preview = pv
			a.publish()
		})
	}()
}

// pastePicture puts a pasted picture, as PNG, with the pictures waiting to go with the next message.
func (a *app) pastePicture(b []byte) {
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		log.Printf("pasted picture: %v", err)
		return
	}
	a.nextID++
	id := "p" + strconv.Itoa(a.nextID)
	a.images[id] = paint.NewImage(img)
	size := img.Bounds().Size()
	a.pending = append(a.pending, Picture{ID: id, W: size.X, H: size.Y})
}

// open makes c the open conversation.
func (a *app) open(c *conv) {
	if c == a.current {
		return
	}
	a.enter(c)
	a.replying, a.editing = "", ""
	a.setTyping("")
	a.setDraft("")
}

// setDraft puts s in the message box.
func (a *app) setDraft(s string) {
	a.text = s
	a.draft = Draft{Text: s, Seq: a.draft.Seq + 1}
}

// submit sends the message box's text as a new message, a reply or an edit.
func (a *app) submit(text string) {
	text = strings.TrimSpace(text)
	if a.editing != "" {
		if text == "" {
			return
		}
		if m, ok := a.current.byID[a.editing]; ok && m.Body != text {
			m.Body, m.Edited = text, true
		}
		a.editing = ""
		a.setDraft("")
		return
	}
	if text == "" && len(a.pending) == 0 {
		return
	}
	m := a.add(a.current, me, text, time.Now().Round(0))
	a.current.readTo = m.ID
	m.ReplyTo, a.replying = a.replying, ""
	m.Pictures, a.pending = a.pending, nil
	a.setDraft("")
	a.send(m)
	a.fetchPreview(m)
	// Somebody usually answers.
	if a.rng.Float64() < 0.6 {
		c := a.current
		a.after(a.between(700*time.Millisecond, 2*time.Second), func() {
			a.say(c, a.pick(c.people), a.pick(answers), m.ID)
		})
	}
}

// send hands one of the user's messages to the server, which takes it after a while, or turns it down. While
// offline it waits, and goes when the connection is back.
func (a *app) send(m *msg) {
	m.State = Pending
	if a.link != Online {
		return
	}
	a.after(a.between(150*time.Millisecond, 900*time.Millisecond), func() {
		if a.link != Online || m.State != Pending {
			return // the connection dropped with the outcome unknown: it goes again on reconnecting
		}
		m.State = Sent
		if a.rng.Float64() < a.failRate {
			m.State = Failed
		}
		a.publish()
	})
}

// toggleLink drops the connection, or starts connecting again and sends what waited.
func (a *app) toggleLink() {
	if a.link == Online {
		a.link = Offline
		a.setTyping("")
		return
	}
	a.link = Reconnecting
	a.after(a.between(800*time.Millisecond, 1600*time.Millisecond), func() {
		a.link = Online
		for _, p := range a.projects {
			for _, c := range p.convs {
				for _, m := range c.msgs {
					if m.State == Pending {
						a.send(m)
					}
				}
			}
		}
		a.publish()
	})
}

// colleague has somebody do something, and comes back after a while to do the next thing.
func (a *app) colleague() {
	defer a.after(a.between(4*time.Second, 11*time.Second), a.colleague)
	if a.link != Online {
		return
	}
	c := a.current
	who := a.pick(c.people)
	switch r := a.rng.Float64(); {
	case r < 0.45:
		a.say(c, who, a.pick(chatter), "")
	case r < 0.56:
		if m := a.lastBy(c, func(m *msg) bool { return m.Author != who }); m != nil {
			a.say(c, who, a.pick(answers), m.ID)
		}
	case r < 0.63:
		// A colleague edits a message once, adding a paragraph.
		if m := a.lastBy(c, func(m *msg) bool { return m.Author == who && !m.Edited }); m != nil {
			m.Body += "\n\n" + a.pick(afterthoughts)
			m.Edited = true
			a.publish()
		}
	case r < 0.7:
		// Somebody votes on a poll they have not voted on.
		if m := a.lastBy(c, func(m *msg) bool { _, voted := m.pollVote(who); return m.poll != nil && !voted }); m != nil {
			m.poll.vote(who, a.rng.IntN(len(m.poll.options)))
			a.publish()
			return
		}
		fallthrough
	case r < 0.83:
		// Somebody reacts, mostly to one of the user's messages.
		m := a.lastBy(c, func(m *msg) bool { return m.Author == me })
		if m == nil || a.rng.Float64() < 0.3 {
			m = a.lastBy(c, func(m *msg) bool { return m.Author != who })
		}
		if m != nil {
			m.toggle(a.pick(reactionEmoji), who)
			a.publish()
		}
	case r < 0.86:
		if m := a.lastBy(c, func(m *msg) bool { return m.Author == who }); m != nil {
			m.Withdrawn = true
			a.publish()
		}
	default:
		// Somewhere else: the message waits unread.
		p := a.projects[a.rng.IntN(len(a.projects))]
		other := p.convs[a.rng.IntN(len(p.convs))]
		if other == a.current {
			return
		}
		a.add(other, a.pick(other.people), a.pick(chatter), time.Now().Round(0))
		other.unread++
		a.publish()
	}
}

// lastBy returns the latest message in c, of the last ten, that keep accepts and that is still there.
func (a *app) lastBy(c *conv, keep func(*msg) bool) *msg {
	for i := len(c.msgs) - 1; i >= 0 && i >= len(c.msgs)-10; i-- {
		if m := c.msgs[i]; !m.Withdrawn && keep(m) {
			return m
		}
	}
	return nil
}

// say has who type for a moment in c, then send body, replying to replyTo when it is set.
func (a *app) say(c *conv, who, body, replyTo string) {
	if a.link != Online {
		return // nobody's typing reaches a user who is offline
	}
	if c == a.current {
		a.setTyping(who)
	}
	a.after(a.between(1200*time.Millisecond, 3500*time.Millisecond), func() {
		if a.link != Online {
			a.setTyping("")
			return
		}
		if c == a.current {
			a.setTyping("")
		} else {
			c.unread++
		}
		m := a.add(c, who, body, time.Now().Round(0))
		if c == a.current {
			c.readTo = m.ID
		}
		m.ReplyTo = replyTo
		a.publish()
		// The colleague's own app fetches the card for a link they send.
		a.fetchPreview(m)
	})
}

// setTyping shows who is typing in the open conversation.
func (a *app) setTyping(who string) {
	if who == a.typing {
		return
	}
	a.typing = who
	if err := a.c.Patch("chat", Typing{Who: who}); err != nil {
		log.Print(err)
	}
}

// publish sends the window the state as it is now.
func (a *app) publish() {
	if err := a.c.Update("chat", a.state()); err != nil {
		log.Print(err)
	}
}

// state returns what the window shows, in values of its own.
func (a *app) state() Chat {
	s := Chat{Project: a.project, Current: a.current.ID, Title: a.current.Name, Link: a.link, Editing: a.editing,
		Draft: a.draft}
	for _, p := range a.projects {
		unread := 0
		for _, c := range p.convs {
			unread += c.unread
		}
		s.Projects = append(s.Projects, Project{Name: p.Name, Short: p.Short, Unread: unread})
	}
	for _, c := range a.projects[a.project].convs {
		s.Conversations = append(s.Conversations, Conversation{ID: c.ID, Name: c.Name, Direct: c.Direct, Unread: c.unread})
	}
	if m, ok := a.current.byID[a.replying]; ok {
		s.Replying = quote(m)
	}
	s.Items = timeline(a.current, time.Now(), a.newFrom)
	if a.newFrom != "" {
		s.NewKey, s.Unread = newKey(a.current), a.unread
	}
	s.Pending, s.Images = slices.Clone(a.pending), maps.Clone(a.images)
	return s
}

// groupFor is how long a message shares the heading of the one that started its group, by the same author.
const groupFor = 5 * time.Minute

// newKey is the key of the line over c's new messages.
func newKey(c *conv) string { return "new:" + c.ID }

// timeline returns c's messages as the timeline's rows, with a heading for each day. A message shares the heading
// of the one before when both are by the same author and its group started under groupFor ago, so even a steady
// stream from one person shows a heading every few minutes. The line over new messages goes before newFrom.
func timeline(c *conv, now time.Time, newFrom string) []Item {
	out := make([]Item, 0, len(c.msgs)+len(c.msgs)/20)
	var prev *msg
	var groupAt time.Time
	day := ""
	for _, m := range c.msgs {
		if d := m.At.Format(time.DateOnly); d != day {
			day, prev = d, nil
			out = append(out, Item{Key: "day:" + c.ID + ":" + d, Day: dayName(m.At, now)})
		}
		if m.ID == newFrom {
			out = append(out, Item{Key: newKey(c), New: true})
			prev = nil
		}
		it := Item{Key: m.ID, Message: Message{ID: m.ID, Author: m.Author, Mine: m.Author == me, At: m.At, Body: m.Body,
			State: m.State, Edited: m.Edited, Withdrawn: m.Withdrawn, Pictures: slices.Clone(m.Pictures),
			Reactions: reactionsOf(m), Preview: m.preview, Poll: pollOf(m)}}
		it.Continued = prev != nil && prev.Author == m.Author && m.At.Sub(groupAt) < groupFor && m.ReplyTo == ""
		if !it.Continued {
			groupAt = m.At
		}
		if q, ok := c.byID[m.ReplyTo]; ok {
			it.Reply = quote(q)
		}
		out = append(out, it)
		prev = m
	}
	return out
}

func quote(m *msg) Quote { return Quote{ID: m.ID, Author: m.Author, Text: m.Body, Gone: m.Withdrawn} }

// dayName names the day of t: today, yesterday, or its weekday and date.
func dayName(t, now time.Time) string {
	y1, m1, d1 := t.Date()
	y2, m2, d2 := now.Date()
	switch {
	case y1 == y2 && m1 == m2 && d1 == d2:
		return "Today"
	case t.Add(24*time.Hour).Format(time.DateOnly) == now.Format(time.DateOnly):
		return "Yesterday"
	}
	return t.Format("Monday 2 January")
}

var chatter = []string{
	"The build runner is pinned to 17.4 and the server is on 17.6. Bump it after the release?",
	"Pushed a fix for the **thumbnail cache**. Can someone look at `thumbs.go`?",
	"Lunch at 12?",
	"The nightly build failed again. Looks like the Windows runner ran out of disk.",
	"I'll take the review.\nTwo small things in the diff, otherwise fine.",
	"Does anyone know why the Linux test is flaky? It passes every time on my machine.",
	"Merged 👍",
	"Release checklist:\n- [ ] update the changelog\n- [ ] tag v5.3.0\n- [ ] tell the customers",
	"Review please: https://example.com/merge_requests/412",
	"Meeting moved to 14:00.",
	"Can we talk about the sync protocol tomorrow? I have some thoughts on the event IDs.",
	"Sounds good.",
	"The customer in Lund says the export is slow on their big datasets. I measured it: most of the time goes to " +
		"sorting the rows before we write them, which we could skip when the view is already in order. I'll try " +
		"that and see how far it gets us.",
	"> the export is slow\nIs that the CSV one or the report?",
	"```\ngo test ./... -run TestSync -count 50\n```\nfails about one time in twenty for me.",
	"Back in an hour.",
	"Thanks!",
	"/poll Where do we go for the team lunch on Friday? | Thai | Pizza | Sushi | Something new",
	"Good read on how range over functions works: https://go.dev/blog/range-functions",
	"Build times this week:\n\n| Runner | Mon | Fri |\n|:--|--:|--:|\n| Windows | 14 min | 9 min |\n| Linux | 6 min | 6 min |",
	"The flaky one:\n```\nwidget/virtual_test.go:377: offset 4712, want the end at 6392 (TestVirtualListThatSticksFollowsNewRows, seed 1817263)\n```",
}

var answers = []string{
	"Good point.",
	"Agreed, let's do that.",
	"Hmm, not sure. Let me check.",
	"On it.",
	"Can you say more about that?",
	"Yes, after lunch.",
	"I think Erik knows more about that one.",
}

var afterthoughts = []string{
	"Edit: make that 15:00.",
	"(Fixed the link.)",
	"Actually, never mind the second point.",
}
