// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

// gunim change: this file is gunim's. macOS keeps its own title bar for
// now: a chromeless window there is an ordinary one, and says so.

package glfw

func (w *Window) platformSetChromeless(on bool) error { return nil }

func (w *Window) platformChromeless() bool { return false }

func (w *Window) platformStartMoveResize(direction int) error { return nil }
