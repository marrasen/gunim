// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

package glfw

// platformSetInputRegion does nothing: an NSWindow ignores the mouse whole or not at all, so it goes on taking the
// pointer everywhere.
func (w *Window) platformSetInputRegion() error { return nil }
