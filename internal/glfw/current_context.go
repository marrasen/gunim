// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

//go:build darwin || freebsd || linux || netbsd || windows

package glfw

import "sync"

// gunim change: the current context is tracked per OS thread, as C
// GLFW does with thread-local storage. The Ebitengine port kept one
// global, which suited its one window on one render thread. gunim gives
// every window a render thread of its own, and with a single global
// each thread would release the others' contexts.
var currentContexts sync.Map // thread ID -> *Window

// currentContext returns the window whose context is current on the
// calling thread.
func currentContext() *Window {
	v, _ := currentContexts.Load(threadID())
	w, _ := v.(*Window)
	return w
}

// setCurrentContext records w as current on the calling thread.
func setCurrentContext(w *Window) {
	if w == nil {
		currentContexts.Delete(threadID())
		return
	}
	currentContexts.Store(threadID(), w)
}
