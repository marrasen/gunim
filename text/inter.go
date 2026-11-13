package text

import _ "embed"

// Inter 4.1, regular and semibold, by The Inter Project Authors, under
// the SIL Open Font License 1.1 in fonts/LICENSE-Inter.txt.
var (
	//go:embed fonts/Inter-Regular.ttf
	interRegular []byte
	//go:embed fonts/Inter-SemiBold.ttf
	interSemibold []byte
)

var interFaces = [2]*Face{parseLate(interRegular, "Inter"), parseLate(interSemibold, "Inter SemiBold")}

// Inter returns Inter, the face the touch kit sets text in, at its
// regular weight or semibold. It is built into the program and read
// the first time it sets text, so a program that never uses it pays
// only for the bytes it carries.
func Inter(semibold bool) *Face {
	if semibold {
		return interFaces[1]
	}
	return interFaces[0]
}
