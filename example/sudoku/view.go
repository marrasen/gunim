package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// Colours the game keeps.
var (
	white = rgb(0xff, 0xff, 0xff)
	plum  = rgb(0x4a, 0x10, 0x60)
	gold  = rgb(0xff, 0xd2, 0x3f)
)

// gameRoot is the whole window: the sky behind, the header, the board,
// the tray of candies, the tools, the effects over everything, and the
// card that ends a game.
type gameRoot struct {
	anim.Group
	// world is the world the game is played in, or nil alone.
	world *worldRoot
	state Game
	// seen is the newest event played.
	seen int
	// selected is the cell picked, or -1; armed the candy picked with
	// no cell, to place with each tap after, or 0.
	selected int
	armed    int8
	// notes says candies go in as notes; numbered draws the digits on
	// the candies.
	notes, numbered bool
	sfx             *sfx
	// shake shakes the board, as a wrong candy lands, and phase is how
	// far along its shaking it is.
	shake  *anim.Float
	phase  float64
	fx     *fx
	sky    *sky
	header *header
	board  *board
	tray   *tray
	tools  *tools
	fxNode *fxNode
	card   *card
	// boardAt and trayAt are where the board and tray are in the
	// window, for effects to start from.
	boardAt, trayAt geom.Point
	boardW          float32
	size            geom.Size
}

func newGameRoot(s *sfx) *gameRoot {
	if s == nil {
		s = newSFX(nil, nil)
	}
	r := &gameRoot{selected: -1, numbered: true, fx: newFX(), sfx: s, shake: anim.NewFloat(0)}
	r.Add(r.shake)
	r.fx.boom = func(at geom.Point) { s.firework(r.pan(at)) }
	r.sky = newSky()
	r.header = newHeader(r)
	r.board = newBoard(r)
	r.tray = newTray(r)
	r.tools = newTools(r)
	r.fxNode = &fxNode{f: r.fx}
	r.card = newCard(r)
	return r
}

// show takes the game as it is now, and plays what happened since.
func (r *gameRoot) show(s Game, u *gunim.UI) {
	fresh := s.Round != r.state.Round
	r.state = s
	if fresh {
		r.seen, r.selected, r.armed = 0, -1, 0
		r.card.hide()
	}
	r.board.show(s, fresh)
	r.header.show(s)
	r.tray.show(s, fresh)
	for _, e := range s.Events {
		if e.ID > r.seen {
			r.play(e)
			r.seen = e.ID
		}
	}
	u.Invalidate()
}

// comboWords are what a combo calls out, from three on.
var comboWords = []string{"Sweet!", "Tasty!", "Delicious!", "Divine!", "Sugar rush!"}

// play shows one event.
func (r *gameRoot) play(e Event) {
	switch e.Kind {
	case Placed:
		r.board.land(e.Cell)
		at := r.cellCenter(e.Cell)
		r.sfx.placed(e.Digit, e.Combo, r.pan(at))
		r.header.combo(e.Combo)
		k := candyOf(e.Digit)
		r.fx.burst(at, 12, sparkle, 260, k.color, white, lighter(k.color, 0.5))
		r.fx.say(fmt.Sprintf("+%d", e.Points), at.Sub(geom.Pt(0, r.cell()*0.6)), r.cell()*0.42, gold)
		if e.Combo >= 3 {
			word := comboWords[min(e.Combo-3, len(comboWords)-1)]
			r.fx.shout(word, geom.Pt(r.size.W/2, r.boardAt.Y+r.boardW*0.42), min(r.boardW*0.14, 60), comboColor(e.Combo))
			r.fx.burst(geom.Pt(r.size.W/2, r.boardAt.Y+r.boardW*0.42), 18+6*e.Combo, starBit, 520, gold, k.color, white)
		}
	case Wrong:
		r.board.wrong(e.Cell, e.Digit)
		r.header.breakHeart()
		r.header.combo(0)
		r.sfx.wrong(r.pan(r.cellCenter(e.Cell)))
		r.shake.Jump(1)
		r.shake.Animate(0, anim.Tween{Duration: 450 * time.Millisecond, Ease: anim.Linear})
	case Done:
		r.board.done(e.Unit)
		r.sfx.done(e.Unit)
		for _, c := range units[e.Unit] {
			r.fx.burst(r.cellCenter(c), 3, starBit, 220, gold, white, rgb(0xff, 0x9f, 0xd8))
		}
		cells := units[e.Unit]
		mid := r.cellCenter(cells[4])
		r.fx.say(fmt.Sprintf("+%d", e.Points), mid, r.cell()*0.55, rgb(0x9b, 0xff, 0xe0))
	case DigitDone:
		r.tray.finish(e.Digit)
		if r.armed == e.Digit {
			// None of it is left to place.
			r.armed = 0
		}
		r.sfx.digitDone(e.Digit)
		r.fx.burst(r.trayCenter(e.Digit), 26, starBit, 420, candyOf(e.Digit).color, white, gold)
	case Hinted:
		r.board.hinted(e.Cell)
	case WonEvent:
		r.selected, r.armed = -1, 0
		r.card.won(r.state.Stars, r.state.Score)
		r.fx.rain(r.size.W, 140, confettiColors...)
		r.fx.fireworks(r.size, 7, confettiColors...)
		r.board.celebrate()
		r.board.letPacmanOut()
		r.header.combo(0)
		r.sfx.won()
	case LostEvent:
		r.selected, r.armed = -1, 0
		r.card.lost()
		r.sfx.lost()
		r.shake.Jump(1.6)
		r.shake.Animate(0, anim.Tween{Duration: 800 * time.Millisecond, Ease: anim.Linear})
	}
}

// pan returns where at lies across the window, for a sound to come
// from: -0.6 at the left edge to 0.6 at the right.
func (r *gameRoot) pan(at geom.Point) float32 {
	if r.size.W <= 0 {
		return 0
	}
	return 0.6 * max(-1, min(1, 2*at.X/r.size.W-1))
}

// Step implements [gunim.Animator]: the board shakes while shake runs.
func (r *gameRoot) Step(dt time.Duration) bool {
	moving := r.Group.Step(dt)
	if r.shake.Value() > 0 {
		r.phase += dt.Seconds()
	}
	return moving
}

var confettiColors = []color.NRGBA{
	rgb(0xff, 0x3b, 0x5c), rgb(0xff, 0x93, 0x1f), rgb(0xff, 0xd2, 0x1f), rgb(0x3d, 0xd6, 0x5a),
	rgb(0x2f, 0x8c, 0xff), rgb(0x9b, 0x5c, 0xff), rgb(0xff, 0x5c, 0xc8), rgb(0x1f, 0xd6, 0xd6),
}

// comboColor warms as a combo grows.
func comboColor(n int) color.NRGBA {
	cs := []color.NRGBA{rgb(0xff, 0xe0, 0x5c), rgb(0xff, 0xa6, 0x3b), rgb(0xff, 0x6b, 0x9e), rgb(0xc9, 0x7c, 0xff), rgb(0x7c, 0xf6, 0xff)}
	return cs[min(n-3, len(cs)-1)]
}

// cell returns a cell's side in the window.
func (r *gameRoot) cell() float32 { return cellSize(r.boardW) }

// cellCenter returns the middle of cell c in the window.
func (r *gameRoot) cellCenter(c int) geom.Point {
	cr := cellRect(c, r.boardW)
	return r.boardAt.Add(geom.Pt((cr.Min.X+cr.Max.X)/2, (cr.Min.Y+cr.Max.Y)/2))
}

// trayCenter returns the middle of digit d's slot in the tray, in the
// window.
func (r *gameRoot) trayCenter(d int8) geom.Point {
	sr := r.tray.slot(d)
	return r.trayAt.Add(geom.Pt((sr.Min.X+sr.Max.X)/2, (sr.Min.Y+sr.Max.Y)/2))
}

// tapCell picks cell c, or places the armed candy in it.
func (r *gameRoot) tapCell(c int, u *gunim.UI) {
	if r.state.Won || r.state.Lost {
		return
	}
	if r.armed > 0 && r.state.Cells[c] == 0 {
		r.selected = c
		u.Send(r, Place{Cell: c, Digit: r.armed, Note: r.notes})
		return
	}
	r.armed = 0
	r.selected = c
	r.sfx.tick(r.pan(r.cellCenter(c)))
	if d := r.state.Cells[c]; d != 0 {
		r.board.cheer(d, c)
	}
}

// pick takes candy d from the tray: the candy picked already lets go;
// any other goes into the cell selected, or, with no empty cell
// selected, is picked for the cells tapped after.
func (r *gameRoot) pick(d int8, u *gunim.UI) {
	if r.state.Won || r.state.Lost {
		return
	}
	if r.armed == d {
		// The cell it went in last stays selected, for the next candy
		r.armed = 0
		return
	}
	if r.selected >= 0 && r.state.Cells[r.selected] == 0 {
		// Cell first: a candy picked before for tapping cells lets go,
		// or the next cell tapped would take it.
		r.armed = 0
		u.Send(r, Place{Cell: r.selected, Digit: d, Note: r.notes})
		return
	}
	r.armed, r.selected = d, -1
	r.board.cheer(d, -1)
}

func (r *gameRoot) erase(u *gunim.UI) {
	if r.selected >= 0 {
		u.Send(r, Erase{Cell: r.selected})
	}
}

func (r *gameRoot) undo(u *gunim.UI) { u.Send(r, Undo{}) }

func (r *gameRoot) hint(u *gunim.UI) { u.Send(r, Hint{Cell: r.selected}) }

func (r *gameRoot) toggleNotes() { r.notes = !r.notes }

// Children implements [gunim.Composite].
func (r *gameRoot) Children() []gunim.Node {
	return []gunim.Node{r.sky, r.header, r.board, r.tray, r.tools, r.card, r.fxNode}
}

// Covers implements [gunim.Shaped]: the game takes the pointer while it
// shows, and lets the map have it while that does.
func (r *gameRoot) Covers(geom.Point) bool { return r.world == nil || !r.world.state.Map }

// Layout implements [gunim.Node]: a column on a phone; on a wide
// window, the board on the left and the tray as a pad on the right.
func (r *gameRoot) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	r.size = size
	sky, head, brd, tr, tl, cd, fxn := kids.At(0), kids.At(1), kids.At(2), kids.At(3), kids.At(4), kids.At(5), kids.At(6)
	// The sky, the card and the effects cover the whole window, under
	// a phone's bars; the game lies in what the bars leave.
	for _, k := range []gunim.Child{sky, cd, fxn} {
		k.Layout(gunim.Tight(size))
		k.Place(geom.Point{})
	}
	area := geom.Rect{Max: size.Point()}.Inset(f.Safe)
	room, o := area.Size(), area.Min
	const m, headH, toolsH = 14, 64, 74
	wide := room.W > room.H*1.15
	if wide {
		side := min(room.H-headH-2*m, room.W*0.6)
		padW := min(room.W-side-3*m, side*0.62)
		total := side + m*2 + padW
		x := o.X + (room.W-total)/2
		head.Layout(gunim.Tight(geom.Sz(total, headH)))
		head.Place(geom.Pt(x, o.Y+m/2))
		r.boardAt, r.boardW = geom.Pt(x, o.Y+headH+m), side
		brd.Layout(gunim.Tight(geom.Sz(side, side)))
		brd.Place(r.boardAt)
		r.tray.grid = true
		trayH := padW
		r.trayAt = geom.Pt(x+side+2*m, o.Y+headH+m+(side-trayH-toolsH-m)/2)
		tr.Layout(gunim.Tight(geom.Sz(padW, trayH)))
		tr.Place(r.trayAt)
		tl.Layout(gunim.Tight(geom.Sz(padW, toolsH)))
		tl.Place(geom.Pt(r.trayAt.X, r.trayAt.Y+trayH+m))
		return size
	}
	r.tray.grid = false
	w := min(room.W-2*m, 620)
	trayH := min(w/9*1.45, 92)
	side := min(w, room.H-headH-trayH-toolsH-5*m)
	total := headH + side + trayH + toolsH + 3*m
	y := o.Y + max(m/2, (room.H-total)/2)
	x := o.X + (room.W-w)/2
	head.Layout(gunim.Tight(geom.Sz(w, headH)))
	head.Place(geom.Pt(x, y))
	y += headH + m/2
	r.boardAt, r.boardW = geom.Pt(o.X+(room.W-side)/2, y), side
	brd.Layout(gunim.Tight(geom.Sz(side, side)))
	brd.Place(r.boardAt)
	y += side + m
	r.trayAt = geom.Pt(x, y)
	tr.Layout(gunim.Tight(geom.Sz(w, trayH)))
	tr.Place(r.trayAt)
	y += trayH + m/2
	tl.Layout(gunim.Tight(geom.Sz(w, toolsH)))
	tl.Place(geom.Pt(x, y))
	return size
}

// Paint implements [gunim.Node]: the board shakes as it is told to.
func (r *gameRoot) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for i := range kids.Len() {
		if i == 2 {
			if s := r.shake.Value(); s > 0 {
				dx := float32(math.Sin(r.phase*70)) * 8 * s
				func() {
					defer p.Push(paint.Translate(geom.Pt(dx, 0)))()
					kids.At(i).Paint(p)
				}()
				continue
			}
		}
		kids.At(i).Paint(p)
	}
}

// Handle implements [gunim.Handler]: the game's keys.
func (r *gameRoot) Handle(e input.Event, u *gunim.UI) bool {
	k, ok := e.(input.KeyPress)
	if !ok {
		return false
	}
	move := func(dc, dr int) {
		if r.selected < 0 {
			r.selected = 40
			return
		}
		col, row := (r.selected%9+dc+9)%9, (r.selected/9+dr+9)%9
		r.selected = row*9 + col
	}
	switch {
	case k.Key >= input.Key1 && k.Key <= input.Key9:
		d := int8(k.Key-input.Key1) + 1
		if k.Mods&input.ModShift != 0 {
			was := r.notes
			r.notes = true
			r.pick(d, u)
			r.notes = was
		} else {
			r.pick(d, u)
		}
	case k.Key >= input.KeyKP1 && k.Key <= input.KeyKP9:
		r.pick(int8(k.Key-input.KeyKP1)+1, u)
	case k.Key == input.KeyLeft:
		move(-1, 0)
	case k.Key == input.KeyRight:
		move(1, 0)
	case k.Key == input.KeyUp:
		move(0, -1)
	case k.Key == input.KeyDown:
		move(0, 1)
	case k.Key == input.KeyBackspace || k.Key == input.KeyDelete:
		r.erase(u)
	case k.Key == input.KeyN:
		r.toggleNotes()
	case k.Key == input.KeyZ || k.Key == input.KeyU:
		r.undo(u)
	case k.Key == input.KeyH:
		r.hint(u)
	case k.Key == input.KeyEscape:
		r.selected, r.armed = -1, 0
	default:
		return false
	}
	u.Invalidate()
	return true
}

// sky is the window's background: a candy dusk with bubbles hanging in
// it. They hold still: bubbles moving all over the window would have
// every frame draw all of it again, which a phone's GPU cannot keep up
// with at its refresh rate, where still ones let a frame draw only
// what moves.
type sky struct {
	t       float64
	bubbles [18]bubble
}

type bubble struct {
	x, speed, size, phase float32
	shape                 int
}

func newSky() *sky {
	s := &sky{t: 23}
	for i := range s.bubbles {
		f := float32(i)
		s.bubbles[i] = bubble{
			x:     float32(math.Mod(float64(f)*0.618, 1)),
			speed: 0.02 + 0.015*float32(math.Mod(float64(f)*0.37, 1)),
			size:  18 + 40*float32(math.Mod(float64(f)*0.73, 1)),
			phase: f * 0.41, shape: i % 9,
		}
	}
	return s
}

// Layout implements [gunim.Node].
func (s *sky) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size { return c.Max }

// Paint implements [gunim.Node].
func (s *sky) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	whole := geom.Rect{Max: box.Point()}
	p.RRect(whole, 0, paint.Fill{Gradient: &paint.Gradient{
		From: geom.Pt(0, 0), To: geom.Pt(box.W*0.3, box.H),
		Start: rgb(0x3b, 0x12, 0x8f), End: rgb(0xe8, 0x3e, 0x9c),
	}})
	// A warm glow low down: a soft round light made once, stretched.
	gr := max(box.W, box.H) * 0.6
	p.Image(glowImage(), geom.Rc(box.W*0.5-gr, box.H*0.85-gr*0.6, 2*gr, 1.2*gr), paint.ImageOpts{Opacity: 0.45})
	for _, b := range s.bubbles {
		travel := box.H + 2*b.size
		y := box.H + b.size - float32(math.Mod(s.t*float64(b.speed)*float64(travel)/40+float64(b.phase)*float64(travel), float64(travel)))
		x := b.x*box.W + 18*float32(math.Sin(s.t*0.8+float64(b.phase)*3))
		// Bubbles of every size share one mask, scaled.
		r := geom.Rc(x-20, y-20, 40, 40)
		shapeAt(p, b.shape, r, b.size/40, geom.Point{}, faded(white, 0.08))
	}
}

// glow is a soft round light, warm, fading from its middle to nothing
// at its edge, for the sky to stretch: a blur would cost every frame.
var glow *paint.Image

func glowImage() *paint.Image {
	if glow != nil {
		return glow
	}
	const n = 64
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			dx, dy := (float64(x)+0.5)/n*2-1, (float64(y)+0.5)/n*2-1
			d := math.Min(1, math.Hypot(dx, dy))
			a := (1 - d*d) * (1 - d*d)
			img.SetNRGBA(x, y, color.NRGBA{R: 0xff, G: 0xb3, B: 0x47, A: uint8(255 * a)})
		}
	}
	glow = paint.NewImage(img)
	return glow
}

// header shows the level, the hearts left and the score.
type header struct {
	anim.Group
	root  *gameRoot
	score *anim.Float
	// hearts breaking, and how far each has broken.
	broken [startLives]*anim.Float
	lives  int
	// count is the combo going, and left how long it has to go on, of
	// comboWindow; meter brings its bar in and out.
	count int
	left  float32
	meter *anim.Float
	// press squashes the map's button as it is pressed.
	press    *anim.Float
	pressing bool
}

// mapButton is where the button back to the map lies.
func mapButton() geom.Rect { return geom.Rc(0, 10, 44, 44) }

// Handle implements [gunim.Handler]: the map's button.
func (h *header) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		if e.Button != input.ButtonPrimary || !mapButton().Contains(e.Pos) {
			return false
		}
		h.pressing = true
		h.press.Animate(1, anim.Spring{Response: 0.1, Damping: 1})
	case input.PointerUp:
		if !h.pressing {
			return false
		}
		h.pressing = false
		h.press.Animate(0, anim.Spring{Response: 0.4, Damping: 0.35})
		if mapButton().Contains(e.Pos) {
			h.root.sfx.tick(-0.5)
			u.Send(h.root, ShowMap{})
		}
	default:
		return false
	}
	u.Invalidate()
	return true
}

func newHeader(r *gameRoot) *header {
	h := &header{root: r, score: anim.NewFloat(0), lives: startLives, meter: anim.NewFloat(0), press: anim.NewFloat(0)}
	h.Add(h.score, h.meter, h.press)
	for i := range h.broken {
		h.broken[i] = anim.NewFloat(0)
		h.Add(h.broken[i])
	}
	return h
}

func (h *header) show(s Game) {
	if s.Score < int(h.score.Target()) {
		h.score.Jump(float32(s.Score))
	} else {
		h.score.Animate(float32(s.Score), anim.Spring{Response: 0.7, Damping: 1})
	}
	if s.Lives > h.lives || s.Lives == startLives {
		for _, b := range h.broken {
			b.Jump(0)
		}
	}
	h.lives = s.Lives
}

// combo starts the combo's meter afresh, or ends it with n under 2.
func (h *header) combo(n int) {
	h.count = n
	if n < 2 {
		h.left = 0
		h.meter.Animate(0, anim.Spring{Response: 0.3, Damping: 1})
		return
	}
	h.left = float32(comboWindow.Seconds())
	h.meter.Animate(1, anim.Spring{Response: 0.35, Damping: 0.55})
}

// Step implements [gunim.Animator]: the combo's time runs out.
func (h *header) Step(dt time.Duration) bool {
	if h.left > 0 {
		h.left -= float32(dt.Seconds())
		if h.left <= 0 {
			h.left = 0
			h.meter.Animate(0, anim.Spring{Response: 0.3, Damping: 1})
		}
	}
	return h.Group.Step(dt) || h.left > 0
}

// breakHeart breaks the heart just lost.
func (h *header) breakHeart() {
	i := h.root.state.Lives
	if i >= 0 && i < len(h.broken) {
		h.broken[i].Jump(0)
		h.broken[i].Animate(1, anim.Tween{Duration: 900 * time.Millisecond, Ease: anim.Linear})
	}
}

// Layout implements [gunim.Node].
func (h *header) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size { return c.Max }

// Paint implements [gunim.Node].
func (h *header) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	s := h.root.state
	mb := mapButton()
	func() {
		mid := geom.Pt(mb.Min.X+mb.Size().W/2, mb.Min.Y+mb.Size().H/2)
		defer p.Push(paint.Scale(1-0.12*h.press.Value(), mid))()
		p.ShadowRRect(mb, 22, paint.Solid(faded(white, 0.22)), paint.Shadow{Blur: 6, Offset: geom.Pt(0, 2), Color: faded(plum, 0.3)})
		widget.PaintIcon(p, f.Theme, icon.Map, geom.Rc(mid.X-11, mid.Y-11, 22, 22), white)
	}()
	title := shaped(fmt.Sprintf("Level %d", s.Level), 22, true)
	paintLabel(p, title, geom.Pt(54, 8), white)
	chip := shaped(s.Difficulty.String(), 12, true)
	cr := geom.Rc(56, 38, chip.Advance+16, 20)
	p.RRect(cr, 10, paint.Solid(faded(white, 0.22)))
	chip.Paint(p, geom.Pt(cr.Min.X+8, cr.Min.Y+3), white)

	// The hearts, in the middle: a broken one splits and falls apart.
	const hs = 30
	hx := box.W/2 - (hs*startLives+8*(startLives-1))/2
	for i := range startLives {
		r := geom.Rc(hx+float32(i)*(hs+8), 16, hs, hs)
		if i < s.Lives {
			p.Mask(candyMask{shape: shapeHeart}, r.Add(geom.Pt(0, 2)), faded(plum, 0.4))
			p.Mask(candyMask{shape: shapeHeart}, r, rgb(0xff, 0x3b, 0x6b))
			shapeAt(p, shapeHeart, r, 0.72, geom.Pt(0, -1), rgb(0xff, 0x7a, 0x9a))
			continue
		}
		p.Mask(candyMask{shape: shapeHeart}, r, faded(white, 0.18))
		if b := h.broken[i].Value(); b > 0 && b < 1 {
			for side := range 2 {
				dir := float32(side*2 - 1)
				half := geom.Rc(r.Min.X+float32(side)*hs/2, r.Min.Y, hs/2, hs)
				fall := b * b * 60
				func() {
					defer p.Push(paint.Translate(geom.Pt(dir*b*14, fall)))()
					defer p.Push(paint.Rotate(dir*b*0.9, geom.Pt(r.Min.X+hs/2, r.Max.Y)))()
					end := p.Layer(paint.LayerOpts{Bounds: half, Opacity: 1 - b, Clip: true})
					p.Mask(candyMask{shape: shapeHeart}, r, rgb(0xff, 0x3b, 0x6b))
					end()
				}()
			}
		}
	}

	// The combo's meter under the hearts: a candy stripe that runs out.
	if m := h.meter.Value(); m > 0.01 {
		mw := float32(hs*startLives+8*(startLives-1)) + 20
		mr := geom.Rc(box.W/2-mw/2, 52, mw, 10)
		func() {
			defer p.Push(paint.Scale(m, geom.Pt(box.W/2, 57)))()
			p.RRect(mr, 5, paint.Solid(faded(plum, 0.5)))
			frac := h.left / float32(comboWindow.Seconds())
			fill := geom.Rc(mr.Min.X, mr.Min.Y, max(mr.Size().W*frac, 10), 10)
			p.RRect(fill, 5, paint.Fill{Gradient: &paint.Gradient{
				From: fill.Min, To: geom.Pt(fill.Max.X, fill.Min.Y),
				Start: comboColor(3), End: comboColor(max(h.count+1, 3)),
			}})
			p.RRect(geom.Rc(fill.Min.X+3, fill.Min.Y+2, fill.Size().W-6, 3), 1.5, paint.Solid(faded(white, 0.45)))
			x := shaped("x"+strconv.Itoa(h.count), 14, true)
			paintLabel(p, x, geom.Pt(mr.Max.X+6, 49), gold)
		}()
	}

	score := shaped(strconv.Itoa(int(math.Round(float64(h.score.Value())))), 26, true)
	paintLabel(p, score, geom.Pt(box.W-score.Advance-6, 8), gold)
	label := shaped("score", 12, true)
	label.Paint(p, geom.Pt(box.W-label.Advance-6, 42), faded(white, 0.7))
}

// paintLabel draws run from at, with a candy edge under it.
func paintLabel(p *paint.Painter, run interface {
	Paint(*paint.Painter, geom.Point, color.NRGBA)
}, at geom.Point, c color.NRGBA) {
	run.Paint(p, at.Add(geom.Pt(0, 2)), faded(plum, 0.7))
	run.Paint(p, at, c)
}

// tray holds a candy for each digit, with how many are left to place.
type tray struct {
	anim.Group
	root *gameRoot
	// grid lays the candies three by three, beside the board.
	grid bool
	// press squashes a candy as it is tapped, lift raises the armed one,
	// and gone bursts and shrinks a digit used up.
	press, lift, gone [9]*anim.Float
	size              geom.Size
	down              int
}

func newTray(r *gameRoot) *tray {
	t := &tray{root: r, down: -1}
	for i := range 9 {
		t.press[i], t.lift[i], t.gone[i] = anim.NewFloat(0), anim.NewFloat(0), anim.NewFloat(0)
		t.Add(t.press[i], t.lift[i], t.gone[i])
	}
	return t
}

func (t *tray) show(s Game, fresh bool) {
	for i := range 9 {
		left := 9 - countOf(s.Cells, int8(i+1))
		switch {
		case fresh && left > 0:
			t.gone[i].Jump(0)
		case left > 0 && t.gone[i].Target() > 0:
			// Undo brought one back.
			t.gone[i].Animate(0, anim.Spring{Response: 0.4, Damping: 0.6})
		case fresh && left == 0:
			t.gone[i].Jump(1)
		}
	}
}

// finish bursts digit d's candy, all of it placed.
func (t *tray) finish(d int8) {
	t.gone[d-1].Animate(1, anim.Spring{Response: 0.45, Damping: 0.6})
}

func countOf(g Grid, d int8) int {
	n := 0
	for _, v := range g {
		if v == d {
			n++
		}
	}
	return n
}

// slot returns where digit d's candy sits.
func (t *tray) slot(d int8) geom.Rect {
	i := int(d - 1)
	if t.grid {
		s := t.size.W / 3
		return geom.Rc(float32(i%3)*s, float32(i/3)*s, s, s)
	}
	s := t.size.W / 9
	return geom.Rc(float32(i)*s, 0, s, t.size.H)
}

// Step implements [gunim.Animator].
func (t *tray) Step(dt time.Duration) bool {
	for i := range 9 {
		to := float32(0)
		if t.root.armed == int8(i+1) {
			to = 1
		}
		if t.lift[i].Target() != to {
			t.lift[i].Animate(to, anim.Spring{Response: 0.35, Damping: 0.55})
		}
	}
	return t.Group.Step(dt)
}

// Handle implements [gunim.Handler].
func (t *tray) Handle(e input.Event, u *gunim.UI) bool {
	at := func(p geom.Point) int {
		for d := int8(1); d <= 9; d++ {
			if t.slot(d).Contains(p) {
				return int(d - 1)
			}
		}
		return -1
	}
	switch e := e.(type) {
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		i := at(e.Pos)
		if i < 0 || t.gone[i].Target() > 0 {
			return false
		}
		t.down = i
		t.press[i].Animate(1, anim.Spring{Response: 0.1, Damping: 1})
	case input.PointerUp:
		if t.down < 0 {
			return false
		}
		i := t.down
		t.down = -1
		t.press[i].Animate(0, anim.Spring{Response: 0.4, Damping: 0.35})
		if at(e.Pos) == i {
			t.root.pick(int8(i+1), u)
		}
	default:
		return false
	}
	u.Invalidate()
	return true
}

// Layout implements [gunim.Node].
func (t *tray) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	t.size = c.Max
	return c.Max
}

// Paint implements [gunim.Node].
func (t *tray) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	whole := geom.Rect{Max: box.Point()}
	p.RRect(whole, 22, paint.Solid(faded(white, 0.12)))
	s := t.root.state
	for d := int8(1); d <= 9; d++ {
		i := d - 1
		sr := t.slot(d)
		side := min(sr.Size().W, sr.Size().H) * 0.78
		if !t.grid {
			side = min(sr.Size().W*0.86, sr.Size().H*0.62)
		}
		mid := geom.Pt((sr.Min.X+sr.Max.X)/2, (sr.Min.Y+sr.Max.Y)/2)
		if !t.grid {
			mid.Y = sr.Min.Y + sr.Size().H*0.42
		}
		gone := t.gone[i].Value()
		if gone >= 0.99 {
			// A tick where the candy was.
			p.RRect(geom.Rc(mid.X-side*0.22, mid.Y-side*0.22, side*0.44, side*0.44), side*0.22, paint.Solid(faded(white, 0.18)))
			continue
		}
		lift := t.lift[i].Value()
		press := t.press[i].Value()
		scale := (1 - 0.15*press + 0.12*lift) * (1 + 0.4*gone) * (1 - gone)
		r := geom.Rc(mid.X-side/2, mid.Y-side/2-lift*side*0.18, side, side)
		if lift > 0.01 {
			p.ShadowRRect(geom.Rc(mid.X-side*0.42, mid.Y-side*0.42, side*0.84, side*0.84), side*0.42,
				paint.Solid(faded(white, 0.0)), paint.Shadow{Blur: side * 0.4, Color: faded(gold, 0.8*lift)})
		}
		func() {
			defer p.Push(paint.Scale(scale, geom.Pt(mid.X, mid.Y)))()
			paintCandy(p, d, r, 1-gone, t.root.numbered, f.Scale)
		}()
		// How many are left to place.
		left := 9 - countOf(s.Cells, d)
		if left > 0 && gone < 0.5 {
			n := shaped(strconv.Itoa(left), 12, true)
			bw := max(n.Advance+10, 20)
			var br geom.Rect
			if t.grid {
				br = geom.Rc(r.Max.X-bw*0.8, r.Min.Y-2, bw, 20)
			} else {
				br = geom.Rc(mid.X-bw/2, sr.Max.Y-24, bw, 20)
			}
			p.RRect(br, 10, paint.Solid(faded(plum, 0.75)))
			n.Paint(p, geom.Pt(br.Min.X+(bw-n.Advance)/2, br.Min.Y+3), white)
		}
	}
}

// tools are the buttons under the tray: undo, erase, notes and hint.
type tools struct {
	anim.Group
	root  *gameRoot
	press [5]*anim.Float
	down  int
	size  geom.Size
}

func newTools(r *gameRoot) *tools {
	t := &tools{root: r, down: -1}
	for i := range t.press {
		t.press[i] = anim.NewFloat(0)
		t.Add(t.press[i])
	}
	return t
}

var toolIcons = [5]*icon.Icon{icon.Undo2, icon.Eraser, icon.Pencil, icon.WandSparkles, icon.Volume2}
var toolNames = [5]string{"Undo", "Erase", "Notes", "Hint", "Sound"}

// The Sound button's icon and name for each setting.
var (
	soundIcons = [4]*icon.Icon{icon.Volume2, icon.Music, icon.Sparkles, icon.VolumeX}
	soundNames = [4]string{"Sound", "Music", "Effects", "Silent"}
)

func (t *tools) slot(i int) geom.Rect {
	w := t.size.W / 5
	return geom.Rc(float32(i)*w, 0, w, t.size.H)
}

// Handle implements [gunim.Handler].
func (t *tools) Handle(e input.Event, u *gunim.UI) bool {
	at := func(p geom.Point) int {
		for i := range 5 {
			if t.slot(i).Contains(p) {
				return i
			}
		}
		return -1
	}
	switch e := e.(type) {
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		t.down = at(e.Pos)
		if t.down < 0 {
			return false
		}
		t.press[t.down].Animate(1, anim.Spring{Response: 0.1, Damping: 1})
	case input.PointerUp:
		if t.down < 0 {
			return false
		}
		i := t.down
		t.down = -1
		t.press[i].Animate(0, anim.Spring{Response: 0.4, Damping: 0.35})
		if at(e.Pos) != i {
			break
		}
		switch i {
		case 0:
			t.root.undo(u)
		case 1:
			t.root.erase(u)
		case 2:
			t.root.toggleNotes()
		case 3:
			t.root.hint(u)
		case 4:
			// What plays steps on: music and sounds, music, sounds,
			// neither. It is kept with the progress.
			next := t.root.sfx.mode.next()
			t.root.sfx.setMode(next)
			u.Send(t.root, SetSound{Sound: next})
		}
	default:
		return false
	}
	u.Invalidate()
	return true
}

// Layout implements [gunim.Node].
func (t *tools) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	t.size = c.Max
	return c.Max
}

// Paint implements [gunim.Node].
func (t *tools) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	for i := range 5 {
		sr := t.slot(i)
		d := min(sr.Size().W*0.62, 48)
		mid := geom.Pt((sr.Min.X+sr.Max.X)/2, sr.Min.Y+d/2+4)
		scale := 1 - 0.12*t.press[i].Value()
		lit := i == 2 && t.root.notes
		func() {
			defer p.Push(paint.Scale(scale, mid))()
			r := geom.Rc(mid.X-d/2, mid.Y-d/2, d, d)
			fill := faded(white, 0.2)
			ink := white
			if lit {
				fill, ink = gold, plum
			}
			p.ShadowRRect(r, d/2, paint.Solid(fill), paint.Shadow{Blur: 8, Offset: geom.Pt(0, 3), Color: faded(plum, 0.35)})
			is := d * 0.46
			ic := toolIcons[i]
			if i == 4 {
				ic = soundIcons[t.root.sfx.mode]
			}
			widget.PaintIcon(p, f.Theme, ic, geom.Rc(mid.X-is/2, mid.Y-is/2, is, is), ink)
		}()
		label := toolNames[i]
		if i == 4 {
			label = soundNames[t.root.sfx.mode]
		}
		name := shaped(label, 12, true)
		name.Paint(p, geom.Pt(mid.X-name.Advance/2, mid.Y+d/2+4), faded(white, 0.85))
	}
	_ = box
}
