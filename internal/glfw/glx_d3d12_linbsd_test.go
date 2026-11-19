// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

//go:build freebsd || linux || netbsd

package glfw

import "testing"

func TestOnlyMesaD3D12Before23_3OverrunsContextIDs(t *testing.T) {
	for _, c := range []struct {
		device  string
		version [3]uint32
		want    bool
	}{
		{"D3D12 (NVIDIA GeForce RTX 3070)", [3]uint32{23, 2, 1}, true},
		{"D3D12 (Intel(R) UHD Graphics)", [3]uint32{22, 3, 6}, true},
		{"D3D12 (NVIDIA GeForce RTX 3070)", [3]uint32{23, 3, 0}, false},
		{"D3D12 (NVIDIA GeForce RTX 3070)", [3]uint32{24, 0, 0}, false},
		{"llvmpipe (LLVM 15.0.7, 256 bits)", [3]uint32{23, 2, 1}, false},
		{"Mesa Intel(R) Graphics (LNL)", [3]uint32{23, 2, 1}, false},
	} {
		if got := d3d12OverrunsContextIDs(c.device, c.version); got != c.want {
			t.Errorf("d3d12OverrunsContextIDs(%q, %v) = %v, want %v", c.device, c.version, got, c.want)
		}
	}
}
