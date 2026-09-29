//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	pRegCreateKeyEx = advapi32.NewProc("RegCreateKeyExW")
	pRegSetValueEx  = advapi32.NewProc("RegSetValueExW")
	pRegDeleteValue = advapi32.NewProc("RegDeleteValueW")
	pRegCloseKey    = advapi32.NewProc("RegCloseKey")
)

const (
	hkeyCurrentUser = 0x80000001
	keySetValue     = 0x0002
	regSZ           = 1
)

func setAutoStart(enabled bool) error {
	var key uintptr
	sub := u16(`Software\Microsoft\Windows\CurrentVersion\Run`)
	r, _, _ := pRegCreateKeyEx.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(sub)), 0, 0, 0, keySetValue, 0, uintptr(unsafe.Pointer(&key)), 0)
	if r != 0 {
		return fmt.Errorf("RegCreateKeyEx: %d", r)
	}
	defer pRegCloseKey.Call(key)
	name := u16("WorkMate")
	if !enabled {
		rr, _, _ := pRegDeleteValue.Call(key, uintptr(unsafe.Pointer(name)))
		if rr != 0 && rr != 2 {
			return fmt.Errorf("RegDeleteValue: %d", rr)
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	value := `"` + exe + `" --background`
	data, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}
	rr, _, _ := pRegSetValueEx.Call(key, uintptr(unsafe.Pointer(name)), 0, regSZ, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)*2))
	if rr != 0 {
		return fmt.Errorf("RegSetValueEx: %d", rr)
	}
	return nil
}
