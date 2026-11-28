package filemanager

// memory is the most the window's pictures take, as Options.Memory or
// Hub.SetMemory says.
type memory struct {
	// thumbs is the most the thumbnails kept take, in bytes, of all the
	// windows that share it; image the largest picture file the preview
	// reads whole; pixels the most pixels a picture may have to be
	// decoded; and workers how many pictures are made thumbnails of at
	// once.
	thumbs, image, pixels int64
	workers               int
	// gate, when set, lets one picture be read and decoded at a time, of
	// all the windows that share it: for a thumbnail, the preview or the
	// viewer.
	gate chan struct{}
}

// defaultMemory is what a window takes where no memory is set.
var defaultMemory = memory{thumbs: thumbBudget, image: maxImage, pixels: maxPixels, workers: workers()}

// memoryOf is what windows may take of most bytes together, and the
// defaults for 0.
//
// The thumbnails kept take a quarter. One picture at a time is read and
// decoded, of at most most/16 pixels, which take four bytes each, so a
// quarter; its file, read whole for the preview, is at most an eighth;
// and what is drawn of it, scaled down, is no larger than the picture.
// So at the most: a quarter, an eighth, a quarter and a quarter, seven
// eighths of most, besides the listing's rows.
func memoryOf(most int64) memory {
	if most <= 0 {
		return defaultMemory
	}
	return memory{
		thumbs: min(most/4, int64(thumbBudget)), image: min(most/8, int64(maxImage)),
		pixels: min(most/16, int64(maxPixels)), workers: 1, gate: make(chan struct{}, 1),
	}
}

// hold waits until a picture may be read and decoded, and returns what
// lets the next one go.
func (m memory) hold() func() {
	if m.gate == nil {
		return func() {}
	}
	m.gate <- struct{}{}
	return func() { <-m.gate }
}

// SetMemory has the windows the hub opens from now on take about most
// bytes for their pictures, all of them together, as Options.Memory does
// for one window: they keep their thumbnails within it between them, and
// decode one picture at a time. It goes before Options.Memory.
func (h *Hub) SetMemory(most int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mem = memoryOf(most)
}

// thumbsShare is the most window a's thumbnails take: its memory's, or
// where its hub's windows share one, its share of that.
func (a *app) thumbsShare() int64 {
	h := a.hub
	if h == nil {
		return a.mem.thumbs
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.mem.gate == nil || h.mem.gate != a.mem.gate {
		return a.mem.thumbs
	}
	n := 0
	for _, o := range h.apps {
		if o.mem.gate == a.mem.gate {
			n++
		}
	}
	return a.mem.thumbs / int64(max(n, 1))
}
