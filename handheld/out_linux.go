//go:build linux

package handheld

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// OpenFramebuffer opens the device and maps it; where the driver does not allow a mapping, frames are written
// with write(2) instead. Every step that fails says which one it was.
func OpenFramebuffer(path string) (*Framebuffer, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	var v [40]uint32 // struct fb_var_screeninfo starts: xres, yres, xres_virtual, yres_virtual, xoffset, yoffset, bits_per_pixel
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), 0x4600, uintptr(unsafe.Pointer(&v[0]))); e != 0 {
		f.Close()
		return nil, fmt.Errorf("FBIOGET_VSCREENINFO: %w", e)
	}
	var fix [20]uint32 // struct fb_fix_screeninfo on 64 bit: id[16], smem_start (8), smem_len, type, type_aux, visual, 3 x u16 steps (8 with padding), line_length
	fixErr := error(nil)
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), 0x4602, uintptr(unsafe.Pointer(&fix[0]))); e != 0 {
		fixErr = fmt.Errorf("FBIOGET_FSCREENINFO: %w", e)
		fix = [20]uint32{}
	}
	fb := &Framebuffer{f: f, w: int(v[0]), h: int(v[1]), vw: int(v[2]), vh: int(v[3]), xoff: int(v[4]), yoff: int(v[5]), bpp: int(v[6])}
	if fb.bpp != 16 && fb.bpp != 32 {
		f.Close()
		return nil, fmt.Errorf("a framebuffer with %d bits per pixel (%dx%d) is not supported", fb.bpp, fb.w, fb.h)
	}
	fb.stride = int(fix[12])
	if fb.stride == 0 {
		fb.stride = fb.w * fb.bpp / 8
	}
	fb.smem = int(fix[6])
	fb.fixErr = fixErr
	fb.buf = make([]byte, fb.stride*fb.h)
	// the whole of the memory the driver says it has, else just the visible part
	for _, n := range []int{fb.smem, fb.stride * fb.vh, fb.stride * fb.h} {
		if n <= 0 {
			continue
		}
		if fb.mem, err = syscall.Mmap(int(f.Fd()), 0, n, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED); err == nil {
			fb.mapped = n
			break
		}
		fb.mmapErr = fmt.Errorf("mmap of %d bytes: %w", n, err)
	}
	return fb, nil
}

func unmapFramebuffer(b []byte) { syscall.Munmap(b) }
