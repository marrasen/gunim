package filemanager

import "github.com/marrasen/gunim/widget"

// overviewParts colours a band of the overview strip: each tint's share
// in its own colour, folders first, so a folder of plain files shows as
// well as one of pictures.
func overviewParts(shares []float32) []widget.OverviewPart {
	parts := make([]widget.OverviewPart, 0, len(shares))
	for _, t := range []Tint{TintFolder, TintImage, TintVideo, TintAudio, TintDocument, TintCode, TintArchive,
		TintProgram, TintOther} {
		if int(t) < len(shares) && shares[t] > 0 {
			parts = append(parts, widget.OverviewPart{Share: shares[t], Color: TintToken(t)})
		}
	}
	return parts
}
