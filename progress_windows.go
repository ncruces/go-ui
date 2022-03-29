package zenity

import (
	"context"
	"sync"
	"syscall"
	"unsafe"
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

	wnd       uintptr
	textCtl   uintptr
	progCtl   uintptr
	okBtn     uintptr
	cancelBtn uintptr
	extraBtn  uintptr
	font      font
}

func (d *progressDialog) Text(text string) error {
	select {
	default:
		setWindowText.Call(d.textCtl, strptr(text))
		return nil
	case <-d.done:
		return d.err
	}
}

func (d *progressDialog) Value(value int) error {
	select {
	default:
		sendMessage.Call(d.progCtl, _PBM_SETPOS, uintptr(value), 0)
		if value >= d.max {
			enableWindow.Call(d.okBtn, 1)
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
		setWindowLong.Call(d.progCtl, intptr(_GWL_STYLE), _WS_CHILD|_WS_VISIBLE|_PBS_SMOOTH)
		sendMessage.Call(d.progCtl, _PBM_SETRANGE32, 0, 1)
		sendMessage.Call(d.progCtl, _PBM_SETPOS, 1, 0)
		enableWindow.Call(d.okBtn, 1)
		enableWindow.Call(d.cancelBtn, 0)
		return nil
	case <-d.done:
		return d.err
	}
}

func (d *progressDialog) Close() error {
	sendMessage.Call(d.wnd, _WM_SYSCOMMAND, _SC_CLOSE, 0)
	<-d.done
	if d.err == ErrCanceled {
		return nil
	}
	return d.err
}

func (dlg *progressDialog) setup(opts options) error {
	done := false
	defer func() {
		if !done {
			dlg.init.Done()
		}
	}()

	defer setup()()
	dlg.font = getFont()
	defer dlg.font.delete()

	if opts.ctx != nil && opts.ctx.Err() != nil {
		return opts.ctx.Err()
	}

	instance, _, err := getModuleHandle.Call(0)
	if instance == 0 {
		return err
	}

	cls, err := registerClass(instance, syscall.NewCallback(progressProc))
	if cls == 0 {
		return err
	}
	defer unregisterClass.Call(cls, instance)

	dlg.wnd, _, _ = createWindowEx.Call(_WS_EX_CONTROLPARENT|_WS_EX_WINDOWEDGE|_WS_EX_DLGMODALFRAME,
		cls, strptr(*opts.title),
		_WS_POPUPWINDOW|_WS_CLIPSIBLINGS|_WS_DLGFRAME,
		_CW_USEDEFAULT, _CW_USEDEFAULT,
		281, 133, 0, 0, instance, uintptr(unsafe.Pointer(dlg)))

	dlg.textCtl, _, _ = createWindowEx.Call(0,
		strptr("STATIC"), 0,
		_WS_CHILD|_WS_VISIBLE|_WS_GROUP|_SS_WORDELLIPSIS|_SS_EDITCONTROL|_SS_NOPREFIX,
		12, 10, 241, 16, dlg.wnd, 0, instance, 0)

	var flags uintptr = _WS_CHILD | _WS_VISIBLE | _PBS_SMOOTH
	if opts.maxValue < 0 {
		flags |= _PBS_MARQUEE
	}
	dlg.progCtl, _, _ = createWindowEx.Call(0,
		strptr(_PROGRESS_CLASS),
		0, flags,
		12, 30, 241, 16, dlg.wnd, 0, instance, 0)

	dlg.okBtn, _, _ = createWindowEx.Call(0,
		strptr("BUTTON"), strptr(*opts.okLabel),
		_WS_CHILD|_WS_VISIBLE|_WS_GROUP|_WS_TABSTOP|_BS_DEFPUSHBUTTON|_WS_DISABLED,
		12, 58, 75, 24, dlg.wnd, _IDOK, instance, 0)
	if !opts.noCancel {
		dlg.cancelBtn, _, _ = createWindowEx.Call(0,
			strptr("BUTTON"), strptr(*opts.cancelLabel),
			_WS_CHILD|_WS_VISIBLE|_WS_GROUP|_WS_TABSTOP,
			12, 58, 75, 24, dlg.wnd, _IDCANCEL, instance, 0)
	}
	if opts.extraButton != nil {
		dlg.extraBtn, _, _ = createWindowEx.Call(0,
			strptr("BUTTON"), strptr(*opts.extraButton),
			_WS_CHILD|_WS_VISIBLE|_WS_GROUP|_WS_TABSTOP,
			12, 58, 75, 24, dlg.wnd, _IDNO, instance, 0)
	}

	dlg.layout(getDPI(dlg.wnd))
	centerWindow(dlg.wnd)
	showWindow.Call(dlg.wnd, _SW_NORMAL, 0)
	if opts.maxValue < 0 {
		sendMessage.Call(dlg.progCtl, _PBM_SETMARQUEE, 1, 0)
	} else {
		sendMessage.Call(dlg.progCtl, _PBM_SETRANGE32, 0, uintptr(opts.maxValue))
	}
	dlg.init.Done()
	done = true

	if opts.ctx != nil {
		wait := make(chan struct{})
		defer close(wait)
		go func() {
			select {
			case <-opts.ctx.Done():
				sendMessage.Call(dlg.wnd, _WM_SYSCOMMAND, _SC_CLOSE, 0)
			case <-wait:
			}
		}()
	}

	if err := messageLoop(dlg.wnd); err != nil {
		return err
	}
	if opts.ctx != nil && opts.ctx.Err() != nil {
		return opts.ctx.Err()
	}
	return dlg.err
}

func (d *progressDialog) layout(dpi dpi) {
	font := d.font.forDPI(dpi)
	sendMessage.Call(d.textCtl, _WM_SETFONT, font, 1)
	sendMessage.Call(d.okBtn, _WM_SETFONT, font, 1)
	sendMessage.Call(d.cancelBtn, _WM_SETFONT, font, 1)
	sendMessage.Call(d.extraBtn, _WM_SETFONT, font, 1)
	setWindowPos.Call(d.wnd, 0, 0, 0, dpi.scale(281), dpi.scale(133), _SWP_NOZORDER|_SWP_NOMOVE)
	setWindowPos.Call(d.textCtl, 0, dpi.scale(12), dpi.scale(10), dpi.scale(241), dpi.scale(16), _SWP_NOZORDER)
	setWindowPos.Call(d.progCtl, 0, dpi.scale(12), dpi.scale(30), dpi.scale(241), dpi.scale(16), _SWP_NOZORDER)
	if d.extraBtn == 0 {
		if d.cancelBtn == 0 {
			setWindowPos.Call(d.okBtn, 0, dpi.scale(178), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
		} else {
			setWindowPos.Call(d.okBtn, 0, dpi.scale(95), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
			setWindowPos.Call(d.cancelBtn, 0, dpi.scale(178), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
		}
	} else {
		if d.cancelBtn == 0 {
			setWindowPos.Call(d.okBtn, 0, dpi.scale(95), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
			setWindowPos.Call(d.extraBtn, 0, dpi.scale(178), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
		} else {
			setWindowPos.Call(d.okBtn, 0, dpi.scale(12), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
			setWindowPos.Call(d.extraBtn, 0, dpi.scale(95), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
			setWindowPos.Call(d.cancelBtn, 0, dpi.scale(178), dpi.scale(58), dpi.scale(75), dpi.scale(24), _SWP_NOZORDER)
		}
	}
}

func progressProc(wnd uintptr, msg uint32, wparam uintptr, lparam *unsafe.Pointer) uintptr {
	var dlg *progressDialog
	switch msg {
	case _WM_NCCREATE:
		saveBackRef(wnd, *lparam)
		dlg = (*progressDialog)(*lparam)
	case _WM_NCDESTROY:
		deleteBackRef(wnd)
	default:
		dlg = (*progressDialog)(loadBackRef(wnd))
	}

	switch msg {
	case _WM_DESTROY:
		postQuitMessage.Call(0)

	case _WM_CLOSE:
		dlg.err = ErrCanceled
		destroyWindow.Call(wnd)

	case _WM_COMMAND:
		switch wparam {
		default:
			return 1
		case _IDOK, _IDYES:
			//
		case _IDCANCEL:
			dlg.err = ErrCanceled
		case _IDNO:
			dlg.err = ErrExtraButton
		}
		destroyWindow.Call(wnd)

	case _WM_DPICHANGED:
		dlg.layout(dpi(uint32(wparam) >> 16))

	default:
		res, _, _ := defWindowProc.Call(wnd, uintptr(msg), wparam, uintptr(unsafe.Pointer(lparam)))
		return res
	}

	return 0
}
