package text

import "testing"

func BenchmarkRasterizeEmoji(b *testing.B) {
	data, err := readEmojiFont()
	if err != nil {
		b.Skip(err)
	}
	f, err := Parse(data)
	if err != nil {
		b.Fatal(err)
	}
	mu.Lock()
	gid, _ := f.face.NominalGlyph(0x1F389)
	mu.Unlock()
	for b.Loop() {
		f.RasterizeColor(uint32(gid), 28)
	}
}
