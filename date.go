package zenity

import (
	"time"
)

// Calendar displays the calendar dialog.
//
// Valid options: Title, Width, Height, OKLabel, CancelLabel, ExtraButton,
// Icon, DefaultDate.
//
// May return: ErrCanceled.
func Calendar(text string, options ...Option) (time.Time, error) {
	return calendar(text, applyOptions(options))
}

// DefaultDate returns an Option to set the date.
func DefaultDate(year int, month time.Month, day int) Option {
	return funcOption(func(o *options) {
		o.year, o.month, o.day = &year, month, day
	})
}
