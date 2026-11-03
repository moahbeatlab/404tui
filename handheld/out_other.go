//go:build !linux && !windows

package handheld

import "errors"

// OpenFramebuffer: a framebuffer exists on Linux only; elsewhere use -web or -script.
func OpenFramebuffer(path string) (*Framebuffer, error) {
	return nil, errors.New("the framebuffer is only available on Linux")
}

func unmapFramebuffer(b []byte) {}
