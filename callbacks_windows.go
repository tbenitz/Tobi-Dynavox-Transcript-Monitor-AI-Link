package main

import "syscall"

func sysCallback(f func(uintptr, uintptr, uintptr, uintptr) uintptr) uintptr {
	return syscall.NewCallback(f)
}

func sysSubclass(f func(uintptr, uintptr, uintptr, uintptr, uintptr, uintptr) uintptr) uintptr {
	return syscall.NewCallback(f)
}
