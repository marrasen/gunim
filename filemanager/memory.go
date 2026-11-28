package filemanager

// memory is the most the window's pictures take, as Options.Memory says.
type memory struct {
	// thumbs is the most the thumbnails kept take, in bytes; image the
	// largest picture file the preview reads whole; pixels the most
	// pixels a picture may have to be decoded; and workers how many
	// pictures are made thumbnails of at once.
	thumbs, image, pixels int64
	workers               int
}

// defaultMemory is what a window takes where Options.Memory is not set.
var defaultMemory = memory{thumbs: thumbBudget, image: maxImage, pixels: maxPixels, workers: workers()}

// memoryOf is what a window may take of most bytes, and the defaults for
// 0. A quarter keeps the thumbnails, one picture at a time is decoded,
// as large as most bytes over the four each pixel takes, and the file it
// is read from in the preview is an eighth.
func memoryOf(most int64) memory {
	if most <= 0 {
		return defaultMemory
	}
	return memory{
		thumbs: min(most/4, int64(thumbBudget)), image: min(most/8, int64(maxImage)),
		pixels: min(most/4, int64(maxPixels)), workers: 1,
	}
}
