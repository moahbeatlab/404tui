//go:build !linux && !darwin

package main

// quietStderr does nothing here: the sound libraries of this system do not write to standard error.
func quietStderr(path string) string { return "" }
