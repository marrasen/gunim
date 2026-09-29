package calendar

import (
	"image/color"

	"github.com/marrasen/gunim/theme"
)

// Theme tokens for the calendar's views. Lines, text and the accent come from package widget's tokens.
var (
	// HourHeight is how tall an hour is in a [Days] grid.
	HourHeight = theme.Length("calendar.hour", 48)
	// NowInk is the line across today at the time it is now.
	NowInk = theme.Color("calendar.now", color.NRGBA{R: 0xf0, G: 0x5a, B: 0x4f, A: 0xff})
	// TodayFill tints today's column, and WeekendFill the columns of Saturdays and Sundays.
	TodayFill   = theme.Color("calendar.today", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0x16})
	WeekendFill = theme.Color("calendar.weekend", color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x10})
	// OffHoursFill shades the hours outside a [Days] grid's working day.
	OffHoursFill = theme.Color("calendar.offhours", color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x0e})
	// OtherMonthFill tints the days of a [Month] outside the month it shows.
	OtherMonthFill = theme.Color("calendar.othermonth", color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x30})
	// EventText is the size of the text on an event.
	EventText = theme.Length("calendar.event.text", 12.5)
)

// Light sets the calendar's tokens for a light theme, to add to one.
var Light = []theme.Entry{
	theme.Set(NowInk, color.NRGBA{R: 0xd9, G: 0x3d, B: 0x32, A: 0xff}),
	theme.Set(TodayFill, color.NRGBA{R: 0x2f, G: 0x6f, B: 0xe0, A: 0x10}),
	theme.Set(WeekendFill, color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x06}),
	theme.Set(OffHoursFill, color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x05}),
	theme.Set(OtherMonthFill, color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x07}),
}
