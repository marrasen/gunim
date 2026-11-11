package widget

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/theme"
)

// ForLook returns th changed to follow the user's system settings: text scaled by look.TextScale, animation off
// under look.LessMotion, and the contrast theme's colours under look.Contrast. Pick th by look.Dark first:
//
//	base := widget.Light()
//	if look.Dark {
//	    base = widget.Dark()
//	}
//	th := widget.ForLook(base, look)
func ForLook(th theme.Theme, look gunim.SystemLook) theme.Theme {
	if look.TextScale > 0 && look.TextScale != 1 {
		th = theme.ScaleText(th, look.TextScale)
	}
	if look.LessMotion {
		th = th.With(theme.Set(theme.Scale, 0))
	}
	if look.Contrast != nil {
		th = th.With(Contrast(*look.Contrast)...)
	}
	return th
}

// Contrast gives every colour of gunim's widgets one of a contrast theme's colours. Shadows go, and so do tints:
// a state shows in the theme's highlight, and status in its icon and words.
func Contrast(c gunim.ContrastColors) []theme.Entry {
	var none color.NRGBA
	set := func(v color.NRGBA, tokens ...theme.Token[color.NRGBA]) []theme.Entry {
		out := make([]theme.Entry, len(tokens))
		for i, t := range tokens {
			out[i] = theme.Set(t, v)
		}
		return out
	}
	scrim := c.Window
	scrim.A = 0xa0
	var out []theme.Entry
	for _, group := range [][]theme.Entry{
		// Grounds.
		set(c.Window, Background, CardFill, DialogFill, PanelFill, MenuFill, FieldFill, ToolbarFill, MenubarFill,
			TooltipFill, CurveFill, HistogramFill, OverviewFill, CodeEditorFill, CodeGutterFill, ChordInputFill, Knob,
			ColorPickerCheckLight),
		set(c.ButtonFace, ButtonFill, ChipFill, ChipCap, SegmentedTrack),
		// Text, lines and status.
		set(c.WindowText, Ink, TooltipInk, MenuHint, PaletteHint, TableHeader, ChipLead, AddressCrumbInk,
			AddressChevron, LogViewLead, SliderRowInk, ChordInputPrompt, CodeGutterInk,
			DialogBorder, PanelBorder, MenuBorder, FieldBorder, ToolbarBorder, ChipCapEdge, SplitLine, ProgressTrack,
			SwitchOff, ScrollbarColor, QuoteBar, SliderRestMark, CurveGrid, TreeGuide, TreeBranch, GridPending,
			ColorPickerCheckDark,
			ToastInfoInk, ToastSuccessInk, ToastWarningInk, ToastErrorInk, DialogProblem, DialogDangerInk,
			DropRefusedInk, ChordInputRefused, CodeProblem,
			SyntaxKeyword, SyntaxBuiltin, SyntaxType, SyntaxFunction, SyntaxString, SyntaxNumber, SyntaxComment,
			SyntaxOperator, SyntaxVariable),
		set(c.GrayText, Placeholder),
		set(c.HotLight, LinkInk),
		// Selection, focus, hover and the accent's marks.
		set(c.Highlight, Accent, ListSelected, MenuHot, Selection, FieldPressed, SliderRowActive, ButtonHover,
			ChipHover, MenubarHot, WindowButtonHot, OverviewHover, OverviewMark, OverviewBox, OverviewBoxEdge,
			TableStrong, SplitHot, GridRule, GridCursor, TableCursor, PaletteMark, BandFill, BandEdge, TileCursor,
			TileHover, TileSelected, TreeHover, TreeSelected, CodeLineFill,
			ButtonPrimaryFill, ButtonPrimaryHover, ButtonDangerFill, ButtonDangerHover),
		set(c.HighlightText, ButtonStrongInk, ButtonPrimaryInk, CheckMark),
		// No shadows and no tints.
		set(none, DialogShadow, PanelShadow, MenuShadow, ToolbarShadow, KnobShadow, ButtonShadow, CodeFill),
		set(scrim, Scrim),
	} {
		out = append(out, group...)
	}
	return out
}
