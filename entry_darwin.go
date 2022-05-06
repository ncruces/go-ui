package zenity

import (
	"os"

	"github.com/ncruces/zenity/internal/zenutil"
)

func entry(text string, opts options) (string, error) {
	var data zenutil.Dialog
	data.Text = text
	data.Operation = "displayDialog"
	data.Options.Title = opts.title
	data.Options.Answer = &opts.entryText
	data.Options.Hidden = opts.hideText
	data.Options.Timeout = zenutil.Timeout
	if opts.customIcon != "" {
		_, err := os.Stat(opts.customIcon)
		if err != nil {
			return "", err
		}
		data.IconPath = opts.customIcon
	} else {
		data.Options.Icon = opts.icon.String()
	}
	data.SetButtons(getButtons(true, true, opts))

	out, err := zenutil.Run(opts.ctx, "dialog", data)
	return strResult(opts, out, err)
}
