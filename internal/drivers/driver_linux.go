//go:build linux

package drivers

import (
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
)

// SKEPTICAL FIX: Using a platform-agnostic way to handle the timeval padding
type linuxInputEvent struct {
	Time  syscall.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

type LinuxDriver struct {
	file *os.File
}

func InitDriver() MouseDriver {
	f, err := os.OpenFile("/dev/uinput", os.O_WRONLY|syscall.O_NONBLOCK, 0660)
	if err != nil {
		fmt.Println("[-] Error: uinput access denied. Try: sudo usermod -aG input $USER")
		os.Exit(1)
	}

	// Constants for uinput
	const (
		UI_SET_EVBIT  = 0x40045564
		UI_SET_KEYBIT = 0x40045565
		UI_SET_RELBIT = 0x40045566
		UI_DEV_SETUP  = 0x405c5503
		UI_DEV_CREATE = 0x5501

		EV_KEY    = 0x01
		EV_REL    = 0x02
		BTN_LEFT  = 0x110
		BTN_RIGHT = 0x111
		REL_X     = 0x00
		REL_Y     = 0x01
		REL_WHEEL = 0x08
	)

	// Setup bits
	ioctl(f.Fd(), UI_SET_EVBIT, EV_KEY)
	ioctl(f.Fd(), UI_SET_EVBIT, EV_REL)
	ioctl(f.Fd(), UI_SET_KEYBIT, BTN_LEFT)
	ioctl(f.Fd(), UI_SET_KEYBIT, BTN_RIGHT)
	ioctl(f.Fd(), UI_SET_RELBIT, REL_X)
	ioctl(f.Fd(), UI_SET_RELBIT, REL_Y)
	ioctl(f.Fd(), UI_SET_RELBIT, REL_WHEEL)

	// Modern Setup (UI_DEV_SETUP)
	// We define the struct locally to ensure correct padding
	type uinputSetup struct {
		ID struct {
			Bustype, Vendor, Product, Version uint16
		}
		Name [80]byte
		_    uint32 // ff_effects_max padding
	}

	setup := uinputSetup{}
	setup.ID.Bustype = 0x03 // BUS_USB
	setup.ID.Vendor = 0x1234
	setup.ID.Product = 0x5678
	copy(setup.Name[:], "Sponge Virtual Mouse")

	// Write setup info
	binary.Write(f, binary.LittleEndian, &setup)
	ioctl(f.Fd(), UI_DEV_CREATE, 0)

	return &LinuxDriver{file: f}
}

func (l *LinuxDriver) WriteEvent(typ, code uint16, val int32) {
	ev := linuxInputEvent{
		Type:  typ,
		Code:  code,
		Value: val,
	}
	// Note: We don't actually need to set Time.Sec/Usec; the kernel fills them.
	binary.Write(l.file, binary.LittleEndian, &ev)
}

func (l *LinuxDriver) Move(dx, dy int32) {
	l.WriteEvent(0x02, 0x00, dx) // REL_X
	l.WriteEvent(0x02, 0x01, dy) // REL_Y
	l.WriteEvent(0x00, 0x00, 0)  // SYN_REPORT
}

func (l *LinuxDriver) Button(b string, down bool) {
	var val int32
	if down {
		val = 1
	}
	code := uint16(0x110)
	if b == "right" {
		code = 0x111
	}

	l.WriteEvent(0x01, code, val)
	l.WriteEvent(0x00, 0x00, 0)
}

func (l *LinuxDriver) Scroll(d int32) {
	// SKEPTICAL FIX: Don't normalize to 1/-1. Pass the actual delta
	// so the scroll speed on Linux matches the phone movement.
	l.WriteEvent(0x02, 0x08, d)
	l.WriteEvent(0x00, 0x00, 0)
}

func (l *LinuxDriver) Close() {
	ioctl(l.file.Fd(), 0x5502, 0) // UI_DEV_DESTROY
	l.file.Close()
}

func ioctl(fd, name, data uintptr) {
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, fd, name, data)
}
