package desktop

// own keeps popup w just above its parent p, below any window in front of p. It runs on the main thread.
func own(w, p *Window) error { return w.gw.SetOwner(p.gw) }
