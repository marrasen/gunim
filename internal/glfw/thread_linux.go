// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

package glfw

import "golang.org/x/sys/unix"

func threadID() uint64 { return uint64(unix.Gettid()) }
