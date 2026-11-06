package handheld

import (
	"fmt"
	"github.com/moahbeatlab/404tui/core"
	"image"
	"os"
)

// Framebuffer is a Linux framebuffer (/dev/fb0) that frames are copied to. Not tried on a device yet.
type Framebuffer struct {
	f      *os.File
	mem    []byte
	w, h   int
	vw, vh int // virtual size
	xoff   int
	yoff   int
	bpp    int
	stride int
	smem   int // smem_len of the driver
	buf    []byte
	mapped int

	fixErr, mmapErr error
}

// Info says what was found, for the log.
func (fb *Framebuffer) Info() string {
	how := "write(2)"
	if fb.mem != nil {
		how = fmt.Sprintf("mmap of %d bytes", fb.mapped)
	}
	s := fmt.Sprintf("%dx%d (virtual %dx%d, offset %d,%d) %d bpp, stride %d, smem_len %d; frames by %s", fb.w, fb.h, fb.vw, fb.vh, fb.xoff, fb.yoff, fb.bpp, fb.stride, fb.smem, how)
	if fb.fixErr != nil {
		s += "; " + fb.fixErr.Error()
	}
	if fb.mmapErr != nil {
		s += "; " + fb.mmapErr.Error()
	}
	return s
}

// Show copies a 640x480 frame, centred and cut to the screen, in the pixel format of the device (16 or 32 bits).
func (fb *Framebuffer) Show(img *image.RGBA) error {
	ox, oy := max((fb.w-core.W)/2, 0), max((fb.h-core.H)/2, 0)
	buf := fb.buf
	for y := 0; y < min(core.H, fb.h); y++ {
		for x := 0; x < min(core.W, fb.w); x++ {
			p := img.PixOffset(x, y)
			r, g, b := img.Pix[p], img.Pix[p+1], img.Pix[p+2]
			o := (y+oy)*fb.stride + (x+ox)*fb.bpp/8
			switch fb.bpp {
			case 32:
				buf[o], buf[o+1], buf[o+2], buf[o+3] = b, g, r, 255
			case 16:
				v := uint16(r>>3)<<11 | uint16(g>>2)<<5 | uint16(b>>3)
				buf[o], buf[o+1] = byte(v), byte(v>>8)
			}
		}
	}
	if fb.mem != nil {
		copy(fb.mem, buf)
		return nil
	}
	_, err := fb.f.WriteAt(buf, 0)
	return err
}

func (fb *Framebuffer) Close() {
	if fb.mem != nil {
		unmapFramebuffer(fb.mem)
	}
	fb.f.Close()
}
