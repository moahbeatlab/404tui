package handheld

import (
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// WritePNG saves a frame.
func WritePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// Framebuffer is a Linux framebuffer (/dev/fb0) that frames are copied to. Not tried on a device yet.
type Framebuffer struct {
	f      *os.File
	mem    []byte
	w, h   int
	bpp    int
	stride int
}

// OpenFramebuffer opens and maps the device.
func OpenFramebuffer(path string) (*Framebuffer, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	var v [40]uint32 // struct fb_var_screeninfo starts: xres, yres, xres_virtual, yres_virtual, xoffset, yoffset, bits_per_pixel
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), 0x4600, uintptr(unsafe.Pointer(&v[0]))); e != 0 {
		f.Close()
		return nil, e
	}
	var fix [20]uint32 // struct fb_fix_screeninfo on 64 bit: id[16], smem_start (8), smem_len, type, type_aux, visual, 3 x u16 steps (8 with padding), line_length
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), 0x4602, uintptr(unsafe.Pointer(&fix[0]))); e != 0 {
		f.Close()
		return nil, e
	}
	fb := &Framebuffer{f: f, w: int(v[0]), h: int(v[1]), bpp: int(v[6])}
	fb.stride = int(fix[12])
	if fb.stride == 0 {
		fb.stride = fb.w * fb.bpp / 8
	}
	fb.mem, err = syscall.Mmap(int(f.Fd()), 0, fb.stride*fb.h, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		f.Close()
		return nil, err
	}
	return fb, nil
}

// Show copies a 640x480 frame, centred and cut to the screen, in the pixel format of the device (16 or 32 bits).
func (fb *Framebuffer) Show(img *image.RGBA) {
	ox, oy := max((fb.w-W)/2, 0), max((fb.h-H)/2, 0)
	for y := 0; y < min(H, fb.h); y++ {
		for x := 0; x < min(W, fb.w); x++ {
			p := img.PixOffset(x, y)
			r, g, b := img.Pix[p], img.Pix[p+1], img.Pix[p+2]
			o := (y+oy)*fb.stride + (x+ox)*fb.bpp/8
			switch fb.bpp {
			case 32:
				fb.mem[o], fb.mem[o+1], fb.mem[o+2], fb.mem[o+3] = b, g, r, 255
			case 16:
				v := uint16(r>>3)<<11 | uint16(g>>2)<<5 | uint16(b>>3)
				fb.mem[o], fb.mem[o+1] = byte(v), byte(v>>8)
			}
		}
	}
}

func (fb *Framebuffer) Close() { syscall.Munmap(fb.mem); fb.f.Close() }

// Web serves the screen as a page with buttons, for trying the app in a browser on a computer or a phone.
type Web struct {
	mu  sync.Mutex
	App *App
}

const page = `<!doctype html><meta name=viewport content="width=device-width,initial-scale=1"><title>404h</title>
<style>body{background:#0b0d10;color:#d9dfe6;font:14px monospace;margin:0;display:flex;flex-direction:column;align-items:center}
img{width:min(100vw,640px);image-rendering:pixelated;margin-top:8px}.pad{display:grid;grid-template-columns:repeat(3,56px);gap:6px;margin:12px}
.pad button,.abxy button{height:56px;background:#1c222a;color:#d9dfe6;border:1px solid #2c343e;border-radius:10px;font:bold 16px monospace;touch-action:none}
.abxy{display:grid;grid-template-columns:repeat(3,56px);gap:6px;margin:12px}.row{display:flex;gap:40px;justify-content:center}
p{color:#6f7a87;max-width:640px;padding:0 12px}</style>
<img id=s src=/frame.png><div class=row><div class=pad>
<span></span><button data-k=U>^</button><span></span><button data-k=L><</button><span></span><button data-k=R>></button><span></span><button data-k=D>v</button><span></span></div>
<div class=abxy><span></span><button data-k=X>X</button><span></span><button data-k=Y>Y</button><span></span><button data-k=A>A</button><span></span><button data-k=B>B</button><span></span></div></div>
<p>Keyboard: arrow keys, a b x y. Hold Y and press a key for its second function (Y+left/right changes screen). Tap Y alone for the key map.</p>
<script>
const img=document.getElementById('s');let t=0;function r(){img.src='/frame.png?'+(t++)}
async function send(k,d){await fetch('/key?k='+k+'&d='+d);r()}
const map={ArrowUp:'U',ArrowDown:'D',ArrowLeft:'L',ArrowRight:'R',a:'A',b:'B',x:'X',y:'Y'};const down={};
addEventListener('keydown',e=>{const k=map[e.key];if(!k||down[k])return;down[k]=1;e.preventDefault();send(k,1)});
addEventListener('keyup',e=>{const k=map[e.key];if(!k)return;down[k]=0;send(k,0)});
for(const b of document.querySelectorAll('button')){const k=b.dataset.k;
b.addEventListener('pointerdown',e=>{e.preventDefault();send(k,1)});b.addEventListener('pointerup',()=>send(k,0));b.addEventListener('pointerleave',()=>{});}
setInterval(r,1000);
</script>`

// Serve listens on addr until it fails.
func (w *Web) Serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) { fmt.Fprint(rw, page) })
	mux.HandleFunc("/frame.png", func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		img := w.App.Render()
		w.mu.Unlock()
		rw.Header().Set("Content-Type", "image/png")
		rw.Header().Set("Cache-Control", "no-store")
		png.Encode(rw, img)
	})
	mux.HandleFunc("/key", func(rw http.ResponseWriter, r *http.Request) {
		k, err := parseKey(strings.ToUpper(r.URL.Query().Get("k")))
		if err != nil {
			http.Error(rw, err.Error(), 400)
			return
		}
		w.mu.Lock()
		w.App.Dispatch(Event{Key: k, Down: r.URL.Query().Get("d") == "1"})
		w.mu.Unlock()
	})
	return http.ListenAndServe(addr, mux)
}
