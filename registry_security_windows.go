//go:build windows
package main

import (
    "errors"
    "syscall"
    "unsafe"
)

var (
    pRegOpenKeyEx = advapi32.NewProc("RegOpenKeyExW")
    pRegQueryValueEx = advapi32.NewProc("RegQueryValueExW")
)
const keyQueryValue = 0x0001

func writeSecurityRegistry(name,value string) error {
    var key uintptr
    sub:=u16(`Software\WorkMate\Security`)
    r,_,_:=pRegCreateKeyEx.Call(hkeyCurrentUser,uintptr(unsafe.Pointer(sub)),0,0,0,keySetValue,0,uintptr(unsafe.Pointer(&key)),0)
    if r!=0 { return errors.New("cannot open security registry") }; defer pRegCloseKey.Call(key)
    n:=u16(name); data,_:=syscall.UTF16FromString(value)
    rr,_,_:=pRegSetValueEx.Call(key,uintptr(unsafe.Pointer(n)),0,regSZ,uintptr(unsafe.Pointer(&data[0])),uintptr(len(data)*2)); if rr!=0 { return errors.New("cannot write security registry") }; return nil
}
func readSecurityRegistry(name string)(string,error){
    var key uintptr; sub:=u16(`Software\WorkMate\Security`)
    r,_,_:=pRegOpenKeyEx.Call(hkeyCurrentUser,uintptr(unsafe.Pointer(sub)),0,keyQueryValue,uintptr(unsafe.Pointer(&key))); if r!=0 { return "",errors.New("not found") }; defer pRegCloseKey.Call(key)
    n:=u16(name); var typ uint32; var size uint32=4096; buf:=make([]uint16,size/2)
    rr,_,_:=pRegQueryValueEx.Call(key,uintptr(unsafe.Pointer(n)),0,uintptr(unsafe.Pointer(&typ)),uintptr(unsafe.Pointer(&buf[0])),uintptr(unsafe.Pointer(&size))); if rr!=0 { return "",errors.New("not found") }
    return syscall.UTF16ToString(buf),nil
}
func readMachineGuid()(string,error){
    var key uintptr; sub:=u16(`SOFTWARE\Microsoft\Cryptography`)
    const hklm=0x80000002
    r,_,_:=pRegOpenKeyEx.Call(hklm,uintptr(unsafe.Pointer(sub)),0,keyQueryValue,uintptr(unsafe.Pointer(&key))); if r!=0 { return "",errors.New("not found") }; defer pRegCloseKey.Call(key)
    n:=u16("MachineGuid"); var typ uint32; var size uint32=512; buf:=make([]uint16,size/2)
    rr,_,_:=pRegQueryValueEx.Call(key,uintptr(unsafe.Pointer(n)),0,uintptr(unsafe.Pointer(&typ)),uintptr(unsafe.Pointer(&buf[0])),uintptr(unsafe.Pointer(&size))); if rr!=0 { return "",errors.New("not found") }
    return syscall.UTF16ToString(buf),nil
}
