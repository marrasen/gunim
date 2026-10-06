package glfw

// platformCreateDeferredContext makes a deferred WGL context on the
// calling thread, and reads what it is, as platformCreateWindow does on
// the main thread for any other window.
func (w *Window) platformCreateDeferredContext(ctxconfig *ctxconfig, fbconfig *fbconfig) error {
	if ctxconfig.source != NativeContextAPI {
		return errNoDeferredContext
	}
	if err := initWGL(); err != nil {
		return err
	}
	if err := w.createContextWGL(ctxconfig, fbconfig); err != nil {
		return err
	}
	return w.refreshContextAttribs(ctxconfig)
}
