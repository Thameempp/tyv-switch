//go:build windows

package selector

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	enableProcessedInput        = 0x0001
	enableLineInput             = 0x0002
	enableEchoInput             = 0x0004
	enableVirtualTerminalInput  = 0x0200
	enableVirtualTerminalOutput = 0x0004
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	getConsoleMode = kernel32.NewProc("GetConsoleMode")
	setConsoleMode = kernel32.NewProc("SetConsoleMode")
)

func consoleMode(h syscall.Handle) (uint32, bool) {
	var m uint32
	r, _, _ := getConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&m)))
	return m, r != 0
}

func setMode(h syscall.Handle, m uint32) error {
	r, _, err := setConsoleMode.Call(uintptr(h), uintptr(m))
	if r == 0 {
		return err
	}
	return nil
}

// IsTerminal reports whether both stdin and stdout are console handles.
func IsTerminal() bool {
	_, in := consoleMode(syscall.Handle(os.Stdin.Fd()))
	_, out := consoleMode(syscall.Handle(os.Stdout.Fd()))
	return in && out
}

// MakeRaw switches the console to unbuffered, no-echo input with virtual
// terminal sequences enabled, and returns a function restoring prior modes.
func MakeRaw() (restore func(), err error) {
	hin := syscall.Handle(os.Stdin.Fd())
	hout := syscall.Handle(os.Stdout.Fd())
	inMode, ok := consoleMode(hin)
	if !ok {
		return nil, syscall.EINVAL
	}
	outMode, _ := consoleMode(hout)

	newIn := (inMode &^ (enableLineInput | enableEchoInput | enableProcessedInput)) | enableVirtualTerminalInput
	if err := setMode(hin, newIn); err != nil {
		return nil, err
	}
	_ = setMode(hout, outMode|enableVirtualTerminalOutput)

	return func() {
		_ = setMode(hin, inMode)
		_ = setMode(hout, outMode)
	}, nil
}
