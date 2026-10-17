package main

import (
	"bufio"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// ownConsole says whether this program made its console window: it was started by a double click,
// not from a terminal that was already open. Then the window goes away when the program ends, and
// whatever it said with it.
func ownConsole() bool {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleProcessList")
	var ids [4]uint32
	n, _, _ := proc.Call(uintptr(unsafe.Pointer(&ids[0])), uintptr(len(ids)))
	return n == 1
}

// holdWindow keeps a console window that is ours open until Enter, so a message can be read.
func holdWindow() {
	if !ownConsole() {
		return
	}
	fmt.Fprint(os.Stderr, "\nPress Enter to close this window.")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
