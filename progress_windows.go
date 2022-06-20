package zenity

import (
	"context"
	"sync"
	"syscall"
	"unsafe"

	"github.com/ncruces/zenity/internal/win"
)

func progress(opts options) (ProgressDialog, error) {
	if opts.title == nil {
		opts.title = stringPtr("")
	}
	if opts.okLabel == nil {
		opts.okLabel = stringPtr("OK")
	}
	if opts.cancelLabel == nil {
		opts.cancelLabel = stringPtr("Cancel")
	}
	if opts.maxValue == 0 {
		opts.maxValue = 100
	}
	if opts.ctx == nil {
		opts.ctx = context.Background()
	} else if cerr := opts.ctx.Err(); cerr != nil {
		return nil, cerr
	}

	dlg := &progressDialog{
		done: make(chan struct{}),
		max:  opts.maxValue,
	}
	dlg.init.Add(1)

	go func() {
		dlg.err = dlg.setup(opts)
		close(dlg.done)
	}()

	dlg.init.Wait()
	return dlg, nil
}

type progressDialog struct {
	init sync.WaitGroup
	done chan struct{}
	max  int
	err  error

	wnd       win.HWND
	textCtl   win.HWND
	progCtl   win.HWND
	okBtn     win.HWND
	cancelBtn win.HWND
	extraBtn  win.HWND
	font      font
}

func (d *progressDialog) Text(text string) error {
	select {
	default:
		win.SetWindowText(d.textCtl, strptr(text))
		return nil
	case <-d.done:
		return d.err
	}
}

func (d *progressDialog) Value(value int) error {
	select {
	default:
		win.SendMessage(d.progCtl, win.PBM_SETPOS, uintptr(value), 0)
		if value >= d.max {
			win.EnableWindow(d.okBtn, true)
		}
		return nil
	case <-d.done:
		return d.err
	}
}

func (d *progressDialog) MaxValue() int {
	return d.max
}

func (d *progressDialog) Done() <-chan struct{} {
	return d.done
}

func (d *progressDialog) Complete() error {
	select {
	default:
		win.SetWindowLong(d.progCtl, _GWL_STYLE, _WS_CHILD|_WS_VISIBLE|_PBS_SMOOTH)
		win.SendMessage(d.progCtl, win.PBM_SETRANGE32, 0, 1)
		win.SendMessage(d.progCtl, win.PBM_SETPOS, 1, 0)
		win.EnableWindow(d.okBtn, true)
		win.EnableWindow(d.cancelBtn, false)
		return nil
	case <-d.done:
		return d.err
	}
}

func (d *progressDialog) Close() error {
	win.SendMessage(d.wnd, win.WM_SYSCOMMAND, _SC_CLOSE, 0)
	<-d.done
	if d.err == ErrCanceled {
		return nil
	}
	return d.err
}

func (dlg *progressDialog) setup(opts options) error {
	var once sync.Once
	defer once.Do(dlg.init.Done)

	defer setup()()
	dlg.font = getFont()
	defer dlg.font.delete()
	icon := getIcon(opts.windowIcon)
	defer icon.delete()

	if opts.ctx != nil && opts.ctx.Err() != nil {
		return opts.ctx.Err()
	}

	instance, err := win.GetModuleHandle(nil)
	if err != nil {
		return err
	}

	cls, err := registerClass(instance, icon.handle, syscall.NewCallback(progressProc))
	if err != nil {
		return err
	}
	defer win.UnregisterClass(cls, instance)

	owner, _ := opts.attach.(win.HWND)
	dlg.wnd, _ = win.CreateWindowEx(_WS_EX_CONTROLPARENT|_WS_EX_WINDOWEDGE|_WS_EX_DLGMODALFRAME,
		cls, strptr(*opts.title),
		_WS_POPUPWINDOW|_WS_CLIPSIBLINGS|_WS_DLGFRAME,
		_CW_USEDEFAULT, _CW_USEDEFAULT,
		281, 133, owner, 0, instance, unsafe.Pointer(dlg))

	dlg.textCtl, _ = win.CreateWindowEx(0,
		strptr("STATIC"), nil,
		_WS_CHILD|_WS_VISIBLE|_WS_GROUP|_SS_WORDELLIPSIS|_SS_EDITCONTROL|_SS_NOPREFIX,
		12, 10, 241, 16, dlg.wnd, 0, instance, nil)

	var flags uint32 = _WS_CHILD | _WS_VISIBLE | _PBS_SMOOTH
	if opts.maxValue < 0 {
		flags |= _PBS_MARQUEE
	}
	dlg.progCtl, _ = win.CreateWindowEx(0,
		strptr(_PROGRESS_CLASS),
		nil, flags,
		12, 30, 241, 16, dlg.wnd, 0, instance, nil)

	dlg.okBtn, _ = win.CreateWindowEx(0,
		strptr("BUTTON"), strptr(*opts.okLabel),
		_WS_CHILD|_WS_VISIBLE|_WS_GROUP|_WS_TABSTOP|_BS_DEFPUSHBUTTON|_WS_DISABLED,
		12, 58, 75, 24, dlg.wnd, win.IDOK, instance, nil)
	if !opts.noCancel {
		dlg.cancelBtn, _ = win.CreateWindowEx(0,
			strptr("BUTTON"), strptr(*opts.cancelLabel),
			_WS_CHILD|_WS_VISIBLE|_WS_GROUP|_WS_TABSTOP,
			12, 58, 75, 24, dlg.wnd, win.IDCANCEL, instance, nil)
	}
	if opts.extraButton != nil {
		dlg.extraBtn, _ = win.CreateWindowEx(0,
			strptr("BUTTON"), strptr(*opts.extraButton),
			_WS_CHILD|_WS_VISIBLE|_WS_GROUP|_WS_TABSTOP,
			12, 58, 75, 24, dlg.wnd, win.IDNO, instance, nil)
	}

	dlg.layout(getDPI(dlg.wnd))
	centerWindow(dlg.wnd)
	win.ShowWindow(dlg.wnd, _SW_NORMAL)
	if opts.maxValue < 0 {
		win.SendMessage(dlg.progCtl, win.PBM_SETMARQUEE, 1, 0)
	} else {
		win.SendMessage(dlg.progCtl, win.PBM_SETRANGE32, 0, uintptr(opts.maxValue))
	}
	once.Do(dlg.init.Done)

	if opts.ctx != nil {
		wait := make(chan struct{})
		defer close(wait)
		go func() {
			select {
			case <-opts.ctx.Done():
				win.SendMessage(dlg.wnd, win.WM_SYSCOMMAND, _SC_CLOSE, 0)
			case <-wait:
			}
		}()
	}

	if err := win.MessageLoop(win.HWND(dlg.wnd)); err != nil {
		return err
	}
	if opts.ctx != nil && opts.ctx.Err() != nil {
		return opts.ctx.Err()
	}
	return dlg.err
}

func (d *progressDialog) layout(dpi dpi) {
	font := d.font.forDPI(dpi)
	win.SendMessage(d.textCtl, win.WM_SETFONT, font, 1)
	win.SendMessage(d.okBtn, win.WM_SETFONT, font, 1)
	win.SendMessage(d.cancelBtn, win.WM_SETFONT, font, 1)
	win.SendMessage(d.extraBtn, win.WM_SETFONT, font, 1)
	win.SetWindowPos(d.wnd, 0, 0, 0, dpi.scale(281), dpi.scale(133), _SWP_NOZORDER|_SWP_NOMOVE)
	win.SetWindowPos(d.textCtl, 0, dpi.scale(12), dpi.scale(10), dpi.scale(241), dpi.scale(16), _SWP_NOZORDER)
	win.SetWindowPos(d.progCtl, 0, dpi.scale(12), dpi.scale(30), dpi.scale(241), dpi.scale(16), _SWP_NOZORDER)
	if d.extraBtn == 0 {
		if d.cancelBtn == 0 {
			win.SetWindowPos(d.okBtn, 0, dpi.scale(178), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
		} else {
			win.SetWindowPos(d.okBtn, 0, dpi.scale(95), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
			win.SetWindowPos(d.cancelBtn, 0, dpi.scale(178), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
		}
	} else {
		if d.cancelBtn == 0 {
			win.SetWindowPos(d.okBtn, 0, dpi.scale(95), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
			win.SetWindowPos(d.extraBtn, 0, dpi.scale(178), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
		} else {
			win.SetWindowPos(d.okBtn, 0, dpi.scale(12), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
			win.SetWindowPos(d.extraBtn, 0, dpi.scale(95), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
			win.SetWindowPos(d.cancelBtn, 0, dpi.scale(178), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
		}
	}
}

func progressProc(wnd uintptr, msg uint32, wparam uintptr, lparam *unsafe.Pointer) uintptr {
	var dlg *progressDialog
	switch msg {
	case win.WM_NCCREATE:
		saveBackRef(wnd, *lparam)
		dlg = (*progressDialog)(*lparam)
	case win.WM_NCDESTROY:
		deleteBackRef(wnd)
	default:
		dlg = (*progressDialog)(loadBackRef(wnd))
	}

	switch msg {
	case win.WM_DESTROY:
		postQuitMessage.Call(0)

	case win.WM_CLOSE:
		dlg.err = ErrCanceled
		destroyWindow.Call(wnd)

	case win.WM_COMMAND:
		switch wparam {
		default:
			return 1
		case win.IDOK, win.IDYES:
			//
		case win.IDCANCEL:
			dlg.err = ErrCanceled
		case win.IDNO:
			dlg.err = ErrExtraButton
		}
		destroyWindow.Call(wnd)

	case win.WM_DPICHANGED:
		dlg.layout(dpi(uint32(wparam) >> 16))

	default:
		res, _, _ := defWindowProc.Call(wnd, uintptr(msg), wparam, uintptr(unsafe.Pointer(lparam)))
		return res
	}

	return 0
}
