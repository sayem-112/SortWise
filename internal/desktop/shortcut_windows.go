package desktop

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A .lnk shortcut is written through the Windows shell's ShellLink object,
// called directly: quick, and with no PowerShell or script involved.

var (
	ole32            = windows.NewLazySystemDLL("ole32.dll")
	coInitializeEx   = ole32.NewProc("CoInitializeEx")
	coUninitialize   = ole32.NewProc("CoUninitialize")
	coCreateInstance = ole32.NewProc("CoCreateInstance")

	clsidShellLink = windows.GUID{Data1: 0x00021401, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidShellLinkW  = windows.GUID{Data1: 0x000214f9, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidPersistFile = windows.GUID{Data1: 0x0000010b, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
)

// Method positions in the objects' tables, as Windows defines them.
const (
	methodQueryInterface = 0
	methodRelease        = 2

	linkGetPath           = 3
	linkSetDescription    = 7
	linkSetWorkingDir     = 9
	linkGetArguments      = 10
	linkSetArguments      = 11
	linkSetIconLocation   = 17
	linkSetPath           = 20
	persistLoad           = 5
	persistSave           = 6
	rpcChangedMode        = 0x80010106
	coinitApartmentThread = 0x2
)

type shortcut struct {
	target, args, folder, description string
}

// comObject is a Windows COM object, which starts with a pointer to its
// table of methods.
type comObject struct{ pointer unsafe.Pointer }

func (object comObject) call(method int, args ...uintptr) uintptr {
	table := *(*unsafe.Pointer)(object.pointer)
	function := *(*uintptr)(unsafe.Add(table, method*int(unsafe.Sizeof(uintptr(0)))))
	result, _, _ := syscall.SyscallN(function, append([]uintptr{uintptr(object.pointer)}, args...)...)
	return result
}

func (object comObject) release() { object.call(methodRelease) }

func failed(result uintptr) bool { return int32(result) < 0 }

// withShellLink runs work with a new ShellLink object and its file view, on
// a thread set up for COM.
func withShellLink(work func(link, file comObject) error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	result, _, _ := coInitializeEx.Call(0, coinitApartmentThread)
	switch {
	case result == 0 || result == 1: // ready, or already set up on this thread
		defer coUninitialize.Call()
	case uint32(result) == rpcChangedMode: // set up differently, which also works
	default:
		return fmt.Errorf("could not use the Windows shell (0x%08x)", uint32(result))
	}

	var link unsafe.Pointer
	const inProcess = 0x1
	result, _, _ = coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidShellLink)), 0, inProcess, uintptr(unsafe.Pointer(&iidShellLinkW)), uintptr(unsafe.Pointer(&link)))
	if failed(result) {
		return fmt.Errorf("could not create a shortcut (0x%08x)", uint32(result))
	}
	defer comObject{link}.release()
	var file unsafe.Pointer
	if result := (comObject{link}).call(methodQueryInterface, uintptr(unsafe.Pointer(&iidPersistFile)), uintptr(unsafe.Pointer(&file))); failed(result) {
		return fmt.Errorf("could not save a shortcut (0x%08x)", uint32(result))
	}
	defer comObject{file}.release()
	return work(comObject{link}, comObject{file})
}

func writeShortcut(path string, details shortcut) error {
	return withShellLink(func(link, file comObject) error {
		text := func(value string) *uint16 {
			pointer, _ := windows.UTF16PtrFromString(value)
			return pointer
		}
		target, args, folder, description, location := text(details.target), text(details.args), text(details.folder), text(details.description), text(path)
		steps := []struct {
			method int
			args   []uintptr
		}{
			{linkSetPath, []uintptr{uintptr(unsafe.Pointer(target))}},
			{linkSetArguments, []uintptr{uintptr(unsafe.Pointer(args))}},
			{linkSetWorkingDir, []uintptr{uintptr(unsafe.Pointer(folder))}},
			{linkSetDescription, []uintptr{uintptr(unsafe.Pointer(description))}},
			{linkSetIconLocation, []uintptr{uintptr(unsafe.Pointer(target)), 0}},
		}
		for _, step := range steps {
			if result := link.call(step.method, step.args...); failed(result) {
				return fmt.Errorf("could not set up the shortcut (0x%08x)", uint32(result))
			}
		}
		const remember = 1
		result := file.call(persistSave, uintptr(unsafe.Pointer(location)), remember)
		runtime.KeepAlive(target)
		runtime.KeepAlive(args)
		runtime.KeepAlive(folder)
		runtime.KeepAlive(description)
		runtime.KeepAlive(location)
		if failed(result) {
			return fmt.Errorf("could not save the shortcut to %s (0x%08x)", path, uint32(result))
		}
		return nil
	})
}
