// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

//go:build darwin || freebsd || netbsd

package glfw

// threadID is one shared slot on these platforms for now, which is the
// single-threaded behaviour the Ebitengine port had. Rendering from
// several threads needs a real ID here first.
func threadID() uint64 { return 0 }
