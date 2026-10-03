package audio

import (
	"encoding/binary"
	"math"
)

// putFloats writes samples into p as little-endian float32s.
func putFloats(p []byte, samples []float32) {
	for i, s := range samples {
		binary.LittleEndian.PutUint32(p[4*i:], math.Float32bits(s))
	}
}
