#include "textflag.h"

// See uia_windows_amd64.s. On arm64, floating-point arguments come in
// F0 and up, and the others in R0 and up, each counted apart.

// ElementProviderFromPoint(this, x, y, out): this in R0, x and y in F0
// and F1, out in R1. They move to R0, R1, R2 and R3.
TEXT ·uiaFromPoint(SB),NOSPLIT|NOFRAME,$0
	MOVD	R1, R3
	FMOVD	F0, R1
	FMOVD	F1, R2
	MOVD	·uiaFromPointGo(SB), R9
	B	(R9)

// IRangeValueProvider.SetValue(this, value): value comes in F0, and
// moves to R1.
TEXT ·uiaSetRange(SB),NOSPLIT|NOFRAME,$0
	FMOVD	F0, R1
	MOVD	·uiaSetRangeGo(SB), R9
	B	(R9)

// uiaThunks returns the shims' addresses.
TEXT ·uiaThunks(SB),NOSPLIT,$0-16
	MOVD	$·uiaFromPoint(SB), R0
	MOVD	R0, fromPoint+0(FP)
	MOVD	$·uiaSetRange(SB), R0
	MOVD	R0, setRange+8(FP)
	RET
