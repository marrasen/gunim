#include "textflag.h"

// Two UI Automation methods take floating-point arguments, which reach
// a method in registers Go's callbacks leave unread. These shims move
// them to where a callback reads its next arguments, as their bits, and
// go on to the callback, which returns to UI Automation itself.

// ElementProviderFromPoint(this, x, y, out): x and y come in X1 and X2,
// and move to DX and R8.
TEXT ·uiaFromPoint(SB),NOSPLIT|NOFRAME,$0
	MOVQ	X1, DX
	MOVQ	X2, R8
	MOVQ	·uiaFromPointGo(SB), AX
	JMP	AX

// IRangeValueProvider.SetValue(this, value): value comes in X1, and
// moves to DX.
TEXT ·uiaSetRange(SB),NOSPLIT|NOFRAME,$0
	MOVQ	X1, DX
	MOVQ	·uiaSetRangeGo(SB), AX
	JMP	AX

// uiaThunks returns the shims' addresses.
TEXT ·uiaThunks(SB),NOSPLIT,$0-16
	MOVQ	$·uiaFromPoint(SB), AX
	MOVQ	AX, fromPoint+0(FP)
	MOVQ	$·uiaSetRange(SB), AX
	MOVQ	AX, setRange+8(FP)
	RET
