//go:build windows

package drivers

import (
	"syscall"
	"unsafe"
)

type WinDriver struct {
	user32 *syscall.LazyDLL
	proc   *syscall.LazyProc
}

// MOUSEINPUT represents the Windows C-struct
type mouseInput struct {
	dx          int32
	dy          int32
	mouseData   int32
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// INPUT represents the generic Windows input container
type input struct {
	inputType uint32
	// For 64-bit, there is 4 bytes of padding here to align the next 8-byte field
	_  uint32
	mi mouseInput
}

func InitDriver() MouseDriver {
	lib := syscall.NewLazyDLL("user32.dll")
	return &WinDriver{
		user32: lib,
		proc:   lib.NewProc("SendInput"),
	}
}

func (w *WinDriver) Send(f uint32, x, y, d int32) {
	var i input
	i.inputType = 0 // INPUT_MOUSE
	i.mi = mouseInput{
		dx:          x,
		dy:          y,
		mouseData:   d,
		dwFlags:     f,
		time:        0,
		dwExtraInfo: 0,
	}

	// We send 1 input. The size must be exactly right for the OS architecture.
	w.proc.Call(
		uintptr(1),
		uintptr(unsafe.Pointer(&i)),
		uintptr(unsafe.Sizeof(i)),
	)
}

func (w *WinDriver) Move(dx, dy int32) { w.Send(0x0001, dx, dy, 0) } // MOUSEEVENTF_MOVE
func (w *WinDriver) Scroll(d int32)    { w.Send(0x0800, 0, 0, d) }   // MOUSEEVENTF_WHEEL
func (w *WinDriver) Button(b string, down bool) {
	var f uint32
	if b == "left" {
		if down {
			f = 0x0002
		} else {
			f = 0x0004
		}
	} else {
		if down {
			f = 0x0008
		} else {
			f = 0x0010
		}
	}
	w.Send(f, 0, 0, 0)
}
func (w *WinDriver) Close() {}
