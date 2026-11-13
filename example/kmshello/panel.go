package main

import (
	"fmt"
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/widget"
)

// pin is the PIN that opens the settings. A real panel keeps it out of the program, and checks it in a process of
// its own.
const pin = "2468"

// panel is the view: a stack of screens over a keyboard on the screen. The first screen shows the beds.
type panel struct {
	*widget.Keyboard
	stack  *widget.Stack
	cards  []*bedCard
	screen *bedScreen
	beds   []Bed
	opened bool
}

// newPanel builds the panel showing s.
func newPanel(s Plants) *panel {
	p := &panel{beds: s.Beds}
	title := heading("Plants")
	settings := widget.NewButton("Settings")
	settings.Icon = icon.Settings
	settings.OnClick = func(u *gunim.UI) gunim.Intent {
		p.stack.Push(p.pinScreen(), u)
		return nil
	}
	top := widget.Row(title, widget.NewSpacer(), settings).Grow(title, 1)
	top.Cross = widget.CrossCenter
	kids := make([]gunim.Node, 0, len(s.Beds))
	for i, b := range s.Beds {
		c := newBedCard(p, i, b)
		p.cards = append(p.cards, c)
		kids = append(kids, c)
	}
	cards := widget.Row(kids...)
	cards.Cross = widget.CrossStretch
	for _, c := range kids {
		cards.Grow(c, 1)
	}
	home := widget.Column(top, cards).Grow(cards, 1)
	home.Cross = widget.CrossStretch
	p.stack = widget.NewStack(widget.NewPad(home))
	switch s.Show {
	case "bed", "name":
		p.open(0, nil)
	case "pin":
		p.stack.Push(p.pinScreen(), nil)
	}
	p.Keyboard = widget.NewKeyboard(p.stack)
	return p
}

// show brings the panel up to date with s.
func (p *panel) show(s Plants, u *gunim.UI) {
	p.beds = s.Beds
	for i, c := range p.cards {
		if i < len(s.Beds) {
			c.show(s.Beds[i], u)
		}
	}
	if p.screen != nil && p.screen.bed < len(s.Beds) {
		p.screen.show(s.Beds[p.screen.bed], u)
	}
	if !p.opened && p.screen != nil && s.Show == "name" {
		p.opened = true
		u.Focus(p.screen.name)
	}
	u.Invalidate()
}

// open pushes bed i's own screen.
func (p *panel) open(i int, u *gunim.UI) {
	p.screen = newBedScreen(p, i, p.beds[i])
	p.stack.Push(p.screen.root, u)
}

// heading is a title in the touch kit's size for titles, semibold.
func heading(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Size, l.Face = widget.HeadingSize, widget.BoldFont
	return l
}

// caption is a line of quiet text in the size for captions.
func caption(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Size, l.Color = widget.CaptionSize, widget.Placeholder
	return l
}

// bedCard is a bed on the first screen: its name and status, its moisture as one big figure, and its buttons.
type bedCard struct {
	*widget.Card
	name, figure, word *widget.Label
	mark               *widget.Icon
	water              *widget.Button
}

func newBedCard(p *panel, i int, b Bed) *bedCard {
	c := &bedCard{
		name:   widget.NewLabel(""),
		figure: widget.NewLabel(""),
		word:   widget.NewLabel(""),
		mark:   widget.NewIcon(icon.CircleCheck, ""),
		water:  widget.NewButton("Water now"),
	}
	c.name.Face = widget.BoldFont
	c.figure.Size, c.figure.Face = widget.FigureSize, widget.BoldFont
	c.water.Icon, c.water.Kind = icon.Droplets, widget.ButtonPrimary
	c.water.OnClick = widget.Sends(WaterNow{Bed: i})
	more := widget.NewButton("Schedule")
	more.OnClick = func(u *gunim.UI) gunim.Intent {
		p.open(i, u)
		return nil
	}
	state := widget.Row(c.mark, c.word)
	state.Cross = widget.CrossCenter
	head := widget.Row(c.name, widget.NewSpacer(), state).Grow(c.name, 1)
	head.Cross = widget.CrossCenter
	buttons := widget.Row(c.water, more)
	gap := widget.NewSpacer()
	body := widget.Column(head, c.figure, caption("Soil moisture"), gap, buttons).Grow(gap, 1)
	body.Cross = widget.CrossStretch
	c.Card = widget.NewCard(body)
	c.show(b, nil)
	return c
}

// show brings the card up to date with b.
func (c *bedCard) show(b Bed, u *gunim.UI) {
	word, ic, ink := status(b)
	c.name.Text = b.Name
	c.figure.Text = percent(b.Moisture)
	c.word.Text, c.word.Color = word, ink
	c.mark.Icon, c.mark.Color = ic, ink
	c.water.Disabled = b.Watering
	u.Invalidate()
}

// bedScreen is a bed's own screen: its schedule and its name.
type bedScreen struct {
	bed     int
	root    gunim.Node
	title   *widget.Label
	morning *widget.Switch
	minutes *widget.Slider
	value   *widget.Label
	name    *widget.TextField
}

func newBedScreen(p *panel, i int, b Bed) *bedScreen {
	s := &bedScreen{
		bed:     i,
		title:   heading(""),
		morning: widget.NewSwitch("Water each morning"),
		minutes: widget.NewSlider(1, 20),
		value:   widget.NewLabel(""),
		name:    widget.NewTextField(),
	}
	back := widget.NewButton("Plants")
	back.Icon = icon.ChevronLeft
	back.OnClick = func(u *gunim.UI) gunim.Intent {
		p.stack.Pop(u)
		return nil
	}
	top := widget.Row(back, s.title).Grow(s.title, 1)
	top.Cross = widget.CrossCenter
	s.morning.OnChange = func(on bool, _ *gunim.UI) gunim.Intent { return SetMorning{Bed: i, On: on} }
	s.minutes.Label, s.minutes.Snap = "Minutes of water", 1
	s.minutes.OnChange = func(v float32, u *gunim.UI) gunim.Intent {
		s.value.Text = minutes(int(math.Round(float64(v))))
		u.Invalidate()
		return nil
	}
	s.minutes.OnCommit = func(v float32, _ *gunim.UI) gunim.Intent {
		return SetMinutes{Bed: i, Minutes: int(math.Round(float64(v)))}
	}
	length := widget.Row(widget.NewLabel("Water for"), s.minutes, s.value).Grow(s.minutes, 1)
	length.Cross = widget.CrossCenter
	s.name.Placeholder = "The bed's name"
	s.name.OnCommit = func(text string, _ *gunim.UI) gunim.Intent { return Rename{Bed: i, Name: text} }
	named := widget.Row(widget.NewLabel("Name"), s.name).Grow(s.name, 1)
	named.Cross = widget.CrossCenter
	body := widget.Column(s.morning, length, named)
	body.Cross = widget.CrossStretch
	scroll := widget.NewScroll(body)
	page := widget.Column(top, scroll).Grow(scroll, 1)
	page.Cross = widget.CrossStretch
	s.root = widget.NewPad(page)
	s.show(b, nil)
	s.name.SetText(b.Name, nil)
	return s
}

// show brings the screen up to date with b.
func (s *bedScreen) show(b Bed, u *gunim.UI) {
	s.title.Text = b.Name
	s.morning.SetChecked(b.Morning, u)
	s.minutes.SetValue(float32(b.Minutes), u)
	s.value.Text = minutes(b.Minutes)
}

// minutes says a number of minutes.
func minutes(n int) string {
	if n == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", n)
}

// pinScreen is the PIN pad that opens the settings. A wrong PIN holds the pad for three seconds.
func (p *panel) pinScreen() gunim.Node {
	pad := widget.NewPinPad(len(pin), "Unlock")
	pad.Label = "PIN for the settings"
	pad.OnCommit = func(typed string, u *gunim.UI) gunim.Intent {
		if typed != pin {
			pad.SetWaiting(3*time.Second, u)
			return nil
		}
		p.stack.Push(p.settingsScreen(), u)
		return nil
	}
	back := widget.NewButton("Plants")
	back.Icon = icon.ChevronLeft
	back.OnClick = func(u *gunim.UI) gunim.Intent {
		p.stack.Pop(u)
		return nil
	}
	top := widget.Row(back, heading("Enter the PIN"))
	top.Cross = widget.CrossCenter
	side := widget.Column(top, caption("The PIN is 2468 in this example."))
	page := widget.Row(side, widget.NewSpacer(), pad).Grow(side, 1)
	return widget.NewPad(page)
}

// settingsScreen is what the PIN opens: a reset of every bed, which takes a hold to confirm.
func (p *panel) settingsScreen() gunim.Node {
	reset := widget.NewHoldButton("Reset all beds")
	reset.Icon = icon.RotateCcw
	reset.OnClick = widget.Sends(ResetAll{})
	lock := widget.NewButton("Lock the settings")
	lock.Icon = icon.Lock
	lock.OnClick = func(u *gunim.UI) gunim.Intent {
		// Back past the PIN pad, to the beds.
		p.stack.Pop(u)
		p.stack.Pop(u)
		return nil
	}
	words := widget.NewLabel("Resetting puts both beds back as they came: their names, their schedules and their " +
		"moisture. Hold the button down until its ring is full.")
	gap := widget.NewSpacer()
	page := widget.Column(heading("Settings"), words, reset, gap, lock).Grow(gap, 1)
	return widget.NewPad(page)
}
