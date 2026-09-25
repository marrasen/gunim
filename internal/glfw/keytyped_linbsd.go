// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

// gunim change: this file is gunim's. See keytyped.go.

//go:build freebsd || linux || netbsd

package glfw

// hasTextCodepoint reports whether s holds any text, past control
// characters.
func hasTextCodepoint(s string) bool {
	for _, r := range s {
		if isTextCodepoint(r) {
			return true
		}
	}
	return false
}
