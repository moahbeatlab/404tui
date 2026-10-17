//go:build !windows

package main

// holdWindow is for Windows, where a double click opens a console window that closes with the program.
func holdWindow() {}
