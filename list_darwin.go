package zenity

import (
	"fmt"

	"github.com/ncruces/zenity/internal/zenutil"
)

func list(text string, items []string, opts options) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("%w: empty items list", ErrUnsupported)
	}
	if opts.extraButton != nil {
		return "", fmt.Errorf("%w: extra button", ErrUnsupported)
	}

	var data zenutil.List
	data.Items = items
	data.Options.Prompt = &text
	data.Options.Title = opts.title
	data.Options.OK = opts.okLabel
	data.Options.Cancel = opts.cancelLabel
	data.Options.Default = opts.defaultItems
	data.Options.Empty = !opts.disallowEmpty

	out, err := zenutil.Run(opts.ctx, "list", data)
	return strResult(opts, out, err)
}

func listMultiple(text string, items []string, opts options) ([]string, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: empty items list", ErrUnsupported)
	}
	if opts.extraButton != nil {
		return nil, fmt.Errorf("%w: extra button", ErrUnsupported)
	}

	var data zenutil.List
	data.Items = items
	data.Options.Prompt = &text
	data.Options.Title = opts.title
	data.Options.OK = opts.okLabel
	data.Options.Cancel = opts.cancelLabel
	data.Options.Default = opts.defaultItems
	data.Options.Empty = !opts.disallowEmpty
	data.Options.Multiple = true
	data.Separator = zenutil.Separator

	out, err := zenutil.Run(opts.ctx, "list", data)
	return lstResult(opts, out, err)
}
