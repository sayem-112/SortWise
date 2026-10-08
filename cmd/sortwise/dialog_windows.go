package main

import (
	"syscall"
	"unsafe"
)

// showError shows a Windows message box, the only way to report a problem
// when Sortwise runs without a console.
func showError(title, message string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	box := user32.NewProc("MessageBoxW")
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	messagePtr, _ := syscall.UTF16PtrFromString(message)
	const iconError = 0x10
	_, _, _ = box.Call(0, uintptr(unsafe.Pointer(messagePtr)), uintptr(unsafe.Pointer(titlePtr)), iconError)
}
