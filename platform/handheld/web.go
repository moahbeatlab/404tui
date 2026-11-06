package handheld

import (
	"fmt"
	"image/png"
	"net/http"
	"strings"
	"sync"

	"github.com/moahbeatlab/404tui/core"
)

// Web serves the screen as a page with buttons, for trying the app in a browser on a computer or a phone.
type Web struct {
	mu  sync.Mutex
	App *core.App
}

const page = `<!doctype html><meta name=viewport content="width=device-width,initial-scale=1"><title>404tui</title>
<style>body{background:#0b0d10;color:#d9dfe6;font:14px monospace;margin:0;display:flex;flex-direction:column;align-items:center}
img{width:min(100vw,640px);image-rendering:pixelated;margin-top:8px}.pad{display:grid;grid-template-columns:repeat(3,56px);gap:6px;margin:12px}
.pad button,.abxy button{height:56px;background:#1c222a;color:#d9dfe6;border:1px solid #2c343e;border-radius:10px;font:bold 16px monospace;touch-action:none}
.abxy{display:grid;grid-template-columns:repeat(3,56px);gap:6px;margin:12px}.row{display:flex;gap:40px;justify-content:center}
p{color:#6f7a87;max-width:640px;padding:0 12px}</style>
<img id=s src=/frame.png><div class=row><div class=pad>
<span></span><button data-k=U>^</button><span></span><button data-k=L><</button><span></span><button data-k=R>></button><span></span><button data-k=D>v</button><span></span></div>
<div class=abxy style="grid-template-columns:repeat(3,56px)"><button data-k=[>L1</button><span></span><button data-k=]>R1</button><span></span><button data-k=X>X</button><span></span><button data-k=Y>Y</button><span></span><button data-k=A>A</button><span></span><button data-k=B>B</button><span></span></div></div>
<p>Keyboard: arrow keys, a b x y; PageUp/PageDown (or q/e) are L1/R1, z/c are L2/R2. Hold Y and press a key for its second function (Y+left/right changes screen). Tap Y alone for the key map.</p>
<script>
const img=document.getElementById('s');let t=0;function r(){img.src='/frame.png?'+(t++)}
async function send(k,d){await fetch('/key?k='+k+'&d='+d);r()}
const map={ArrowUp:'U',ArrowDown:'D',ArrowLeft:'L',ArrowRight:'R',a:'A',b:'B',x:'X',y:'Y',PageUp:'[',PageDown:']',q:'[',e:']',z:'{',c:'}'};const down={};
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
		k, err := core.ParseKey(strings.ToUpper(r.URL.Query().Get("k")))
		if err != nil {
			http.Error(rw, err.Error(), 400)
			return
		}
		w.mu.Lock()
		w.App.Dispatch(core.Event{Key: k, Down: r.URL.Query().Get("d") == "1"})
		w.mu.Unlock()
	})
	return http.ListenAndServe(addr, mux)
}
