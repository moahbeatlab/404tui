package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/moahbeatlab/404tui/replay"
)

const renderUsage = `usage: sp404 replay render <audio.wav> [events.jsonl] [options]
  -o out.mp4        the video (default: the WAV's name with .mp4, beside it)
  -format wide|vertical   1920x1080, or 1080x1920 for shorts and reels (default wide)
  -fps 30           frames per second (30 or 60)
  -from, -to        a part of the take, in seconds
  -q "preset=neon&accent=%23ff3b30"   more of the page's parameters (the saved look and title are used as they are)
  -chrome path      the browser (Chrome, Chromium, Edge or Brave; found by itself, or in $CHROME)
  -ffmpeg path      ffmpeg (default: the one in the path)
  -workers 4        pages drawing frames side by side (default: half the processors, at most 4)
  -no-camera        leave the camera video out (a quicker draft)
  -no-sandbox       for a browser that cannot start its sandbox (a container, a root account)
The page is drawn in a headless browser, one picture at a time (so it does not matter how fast the computer is), and ffmpeg
puts the pictures and the sound together.`

// runRender renders a take into a video: sp404 replay render take.wav -o take.mp4.
func runRender(args []string, o options) error {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "-help" || a == "help" {
			fmt.Println(renderUsage)
			return nil
		}
	}
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, renderUsage) }
	out := fs.String("o", "", "")
	format := fs.String("format", "wide", "")
	fps := fs.Int("fps", 30, "")
	from := fs.Float64("from", 0, "")
	to := fs.Float64("to", 0, "")
	query := fs.String("q", "", "")
	chrome := fs.String("chrome", os.Getenv("CHROME"), "")
	ffmpeg := fs.String("ffmpeg", "ffmpeg", "")
	noSandbox := fs.Bool("no-sandbox", false, "")
	noCamera := fs.Bool("no-camera", false, "")
	workers := fs.Int("workers", min(4, max(runtime.NumCPU()/2, 1)), "")
	// the files come first, the options after (the flag package stops at the first file)
	var files []string
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		files, args = append(files, args[0]), args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	files = append(files, fs.Args()...)
	if len(files) < 1 || len(files) > 2 {
		return errors.New(renderUsage)
	}
	if *format != "wide" && *format != "vertical" {
		return fmt.Errorf("-format: wide or vertical, not %q", *format)
	}
	if *fps < 1 || *fps > 120 {
		return fmt.Errorf("-fps %d", *fps)
	}
	events := ""
	if len(files) == 2 {
		events = files[1]
	}
	dest := *out
	if dest == "" {
		dest = strings.TrimSuffix(files[0], filepath.Ext(files[0])) + ".mp4"
	}
	ff, err := exec.LookPath(*ffmpeg)
	if err != nil {
		return fmt.Errorf("ffmpeg not found (install it, or give its path with -ffmpeg): %w", err)
	}
	browser, err := findBrowser(*chrome)
	if err != nil {
		return err
	}

	fmt.Println("reading the recording...")
	r, err := replay.Load(files[0], events)
	if err != nil {
		return err
	}
	pageURL, stop, err := r.Serve("127.0.0.1:0")
	if err != nil {
		return err
	}
	defer stop()
	q := url.Values{}
	q.Set("hud", "0")
	if *format == "vertical" {
		q.Set("format", "vertical")
	}
	pageURL += "?" + q.Encode()
	if *query != "" {
		pageURL += "&" + strings.TrimPrefix(*query, "&")
	}

	start, end := *from, r.Analysis.Duration
	if *to > 0 && *to < end {
		end = *to
	}
	if start < 0 || start >= end {
		return fmt.Errorf("-from %.1f is not inside the take (%.1f s)", start, r.Analysis.Duration)
	}
	// the parts that are cut leave the video: what is rendered is the list of the parts that are left, frame by frame
	spans, times := planFrames(r.Cuts(), start, end, *fps)
	frames := len(times)
	if frames < 1 {
		return errors.New("nothing to render (everything is cut)")
	}
	cut := len(spans) != 1 || spans[0].cutBefore || spans[0].cutAfter || spans[0].n != int((end-start)*float64(*fps))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// the video of the player: its pictures for these frames are cut out with ffmpeg (a picture for every frame of the render, from the
	// time of the camera that matches the start, by the sync of the take) and given to the page one by one
	var camDir string
	if cp := r.CameraPath(); cp != "" && !*noCamera {
		if _, err := os.Stat(cp); err != nil {
			fmt.Printf("the camera video %s is not there: rendering without it\n", cp)
		} else {
			camDir, err = os.MkdirTemp("", "sp404-cam-")
			if err != nil {
				return err
			}
			defer os.RemoveAll(camDir)
			for _, sp := range spans {
				if err = extractCameraFrames(ctx, ff, cp, camDir, cameraSync(r.Camera()), sp.from, *fps, sp.n, sp.first); err != nil {
					return err
				}
			}
		}
	}
	br, err := startBrowser(ctx, browser, *noSandbox)
	if err != nil {
		return err
	}
	defer br.close()
	if *workers < 1 || *workers > 16 {
		return fmt.Errorf("-workers %d", *workers)
	}
	sessions := make([]string, *workers)
	for i := range sessions {
		if sessions[i], err = br.open(pageURL); err != nil {
			return err
		}
	}
	for _, sess := range sessions {
		if err := br.waitReady(sess, 90*time.Second); err != nil {
			return err
		}
	}

	// ffmpeg: the pictures (PNG) from its input, the sound from the WAV (from the same second)
	fa := []string{"-y", "-loglevel", "error", "-f", "image2pipe", "-framerate", fmt.Sprint(*fps), "-c:v", "png", "-i", "-"}
	if cut { // the sound of the parts that are left, joined
		fa = append(fa, "-i", files[0], "-filter_complex", audioFilter(spans, *fps), "-map", "0:v", "-map", "[a]")
	} else {
		if start > 0 {
			fa = append(fa, "-ss", fmt.Sprintf("%.3f", start))
		}
		fa = append(fa, "-i", files[0], "-map", "0:v", "-map", "1:a")
	}
	fa = append(fa, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", "17", "-preset", "medium",
		"-c:a", "aac", "-b:a", "192k", "-t", fmt.Sprintf("%.3f", float64(frames)/float64(*fps)), "-movflags", "+faststart", dest)
	cmd := exec.CommandContext(ctx, ff, fa...)
	cmd.Stderr = os.Stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	fmt.Printf("rendering %d frames (%.1f s, %d fps, %s) to %s\n", frames, float64(frames)/float64(*fps), *fps, *format, dest)
	// the pages draw frames side by side (each in a tab of its own); the pictures go to ffmpeg in their order
	var (
		mu      sync.Mutex
		cond    = sync.NewCond(&mu)
		nextJob int // the next frame a page takes
		nextOut int // the next frame ffmpeg is waiting for
		ready   = map[int][]byte{}
		failed  error
		wg      sync.WaitGroup
	)
	window := *workers * 3 // how far ahead of ffmpeg the pages may go
	began := time.Now()
	for _, sess := range sessions {
		wg.Add(1)
		go func(sess string) {
			defer wg.Done()
			for {
				mu.Lock()
				for failed == nil && nextJob < frames && nextJob >= nextOut+window {
					cond.Wait()
				}
				if failed != nil || nextJob >= frames {
					mu.Unlock()
					return
				}
				i := nextJob
				nextJob++
				mu.Unlock()
				var cam []byte
				if camDir != "" {
					cam, _ = os.ReadFile(filepath.Join(camDir, fmt.Sprintf("c%06d.jpg", i+1))) // none before the video begins or after its end
				}
				var b []byte
				var err error
				if camDir != "" {
					b, err = br.frameWithCamera(sess, times[i], cam)
				} else {
					b, err = br.frame(sess, times[i])
				}
				mu.Lock()
				if err != nil && failed == nil {
					failed = err
				}
				ready[i] = b
				cond.Broadcast()
				mu.Unlock()
			}
		}(sess)
	}
	var writeErr error
	for nextOut < frames {
		mu.Lock()
		for failed == nil && ready[nextOut] == nil {
			cond.Wait()
		}
		if failed != nil {
			mu.Unlock()
			break
		}
		b := ready[nextOut]
		delete(ready, nextOut)
		i := nextOut
		nextOut++
		cond.Broadcast()
		mu.Unlock()
		if _, err := in.Write(b); err != nil {
			writeErr = err
			mu.Lock()
			failed = err
			cond.Broadcast()
			mu.Unlock()
			break
		}
		if i%*fps == 0 || i == frames-1 {
			el := time.Since(began).Seconds()
			rate := float64(i+1) / el
			fmt.Printf("\r  frame %d/%d  %.1f frames/s  about %.0f s left   ", i+1, frames, rate, float64(frames-i-1)/rate)
		}
	}
	wg.Wait()
	in.Close()
	fmt.Println()
	werr := cmd.Wait()
	switch {
	case failed != nil && writeErr == nil:
		return failed
	case writeErr != nil:
		return fmt.Errorf("ffmpeg: %w", writeErr)
	case werr != nil:
		return fmt.Errorf("ffmpeg: %w", werr)
	}
	if st, err := os.Stat(dest); err == nil {
		fmt.Printf("done: %s (%.1f MB) in %.0f s\n", dest, float64(st.Size())/1e6, time.Since(began).Seconds())
	}
	return nil
}

// findBrowser is the Chrome-like browser to draw the page in.
func findBrowser(given string) (string, error) {
	if given != "" {
		if p, err := exec.LookPath(given); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("the browser %q was not found", given)
	}
	var names []string
	switch runtime.GOOS {
	case "darwin":
		names = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
	case "windows":
		for _, d := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")} {
			if d != "" {
				names = append(names, filepath.Join(d, `Google\Chrome\Application\chrome.exe`), filepath.Join(d, `Microsoft\Edge\Application\msedge.exe`))
			}
		}
	default:
		names = []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "brave-browser", "chrome"}
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no Chrome, Chromium, Edge or Brave found: install one, or give it with -chrome (or $CHROME)")
}

// browser is a headless Chrome that is spoken to over the DevTools protocol on two pipes (no extra libraries, no network port).
type browser struct {
	cmd     *exec.Cmd
	to      *os.File
	mu      sync.Mutex
	nextID  int
	pending map[int]chan cdpReply
	dir     string
	dead    chan struct{}
}

type cdpReply struct {
	Result json.RawMessage
	Err    string
}

func startBrowser(ctx context.Context, path string, noSandbox bool) (*browser, error) {
	dir, err := os.MkdirTemp("", "sp404-render-")
	if err != nil {
		return nil, err
	}
	toR, toW, err := os.Pipe() // what Chrome reads (its file 3)
	if err != nil {
		return nil, err
	}
	fromR, fromW, err := os.Pipe() // what Chrome writes (its file 4)
	if err != nil {
		return nil, err
	}
	args := []string{"--headless=new", "--remote-debugging-pipe", "--user-data-dir=" + dir, "--no-first-run", "--no-default-browser-check",
		"--hide-scrollbars", "--mute-audio", "--disable-background-timer-throttling", "--disable-renderer-backgrounding",
		"--disable-backgrounding-occluded-windows", "--force-device-scale-factor=1", "--window-size=1280,720", "about:blank"}
	if noSandbox {
		args = append([]string{"--no-sandbox"}, args...)
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.ExtraFiles = []*os.File{toR, fromW}
	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("could not start %s: %w", path, err)
	}
	toR.Close()
	fromW.Close()
	b := &browser{cmd: cmd, to: toW, pending: map[int]chan cdpReply{}, dir: dir, dead: make(chan struct{})}
	go b.read(fromR)
	return b, nil
}

func (b *browser) read(f *os.File) {
	defer close(b.dead)
	rd := bufio.NewReaderSize(f, 1<<20)
	for {
		msg, err := rd.ReadBytes(0)
		if err != nil {
			b.mu.Lock()
			for id, ch := range b.pending {
				ch <- cdpReply{Err: "the browser closed"}
				delete(b.pending, id)
			}
			b.mu.Unlock()
			return
		}
		var m struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(bytes.TrimRight(msg, "\x00"), &m) != nil || m.ID == 0 {
			continue // an event: nothing here waits for one
		}
		r := cdpReply{Result: m.Result}
		if m.Error != nil {
			r.Err = m.Error.Message
		}
		b.mu.Lock()
		ch := b.pending[m.ID]
		delete(b.pending, m.ID)
		b.mu.Unlock()
		if ch != nil {
			ch <- r
		}
	}
}

func (b *browser) call(session, method string, params any) (json.RawMessage, error) {
	b.mu.Lock()
	b.nextID++
	id := b.nextID
	ch := make(chan cdpReply, 1)
	b.pending[id] = ch
	b.mu.Unlock()
	msg := map[string]any{"id": id, "method": method, "params": params}
	if session != "" {
		msg["sessionId"] = session
	}
	raw, _ := json.Marshal(msg)
	if _, err := b.to.Write(append(raw, 0)); err != nil {
		return nil, fmt.Errorf("the browser is gone: %w", err)
	}
	select {
	case r := <-ch:
		if r.Err != "" {
			return nil, fmt.Errorf("%s: %s", method, r.Err)
		}
		return r.Result, nil
	case <-b.dead:
		return nil, errors.New("the browser closed")
	case <-time.After(60 * time.Second):
		return nil, fmt.Errorf("%s: no answer from the browser", method)
	}
}

// open opens a page (a tab of its own) and gives its session.
func (b *browser) open(pageURL string) (string, error) {
	res, err := b.call("", "Target.createTarget", map[string]any{"url": pageURL})
	if err != nil {
		return "", err
	}
	var t struct {
		TargetID string `json:"targetId"`
	}
	json.Unmarshal(res, &t)
	res, err = b.call("", "Target.attachToTarget", map[string]any{"targetId": t.TargetID, "flatten": true})
	if err != nil {
		return "", err
	}
	var a struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(res, &a)
	return a.SessionID, nil
}

// eval runs an expression in the page and gives its value.
func (b *browser) eval(session, expr string) (json.RawMessage, error) {
	res, err := b.call(session, "Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true})
	if err != nil {
		return nil, err
	}
	var r struct {
		Result    struct{ Value json.RawMessage } `json:"result"`
		Exception *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return nil, err
	}
	if r.Exception != nil {
		return nil, fmt.Errorf("in the page: %s %s", r.Exception.Text, r.Exception.Exception.Description)
	}
	return r.Result.Value, nil
}

// evalAsync runs an expression that gives a promise and waits for what it gives.
func (b *browser) evalAsync(session, expr string) (json.RawMessage, error) {
	res, err := b.call(session, "Runtime.evaluate", map[string]any{"expression": expr, "returnByValue": true, "awaitPromise": true})
	if err != nil {
		return nil, err
	}
	var r struct {
		Result    struct{ Value json.RawMessage } `json:"result"`
		Exception *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(res, &r); err != nil {
		return nil, err
	}
	if r.Exception != nil {
		return nil, fmt.Errorf("in the page: %s %s", r.Exception.Text, r.Exception.Exception.Description)
	}
	return r.Result.Value, nil
}

// frameWithCamera draws the picture of a time with the picture of the camera for it (a JPEG, or none) and gives it as PNG bytes.
func (b *browser) frameWithCamera(session string, t float64, cam []byte) ([]byte, error) {
	arg := "null"
	if len(cam) > 0 {
		arg = `"data:image/jpeg;base64,` + base64.StdEncoding.EncodeToString(cam) + `"`
	}
	v, err := b.evalAsync(session, fmt.Sprintf("window.replay.frameWithCamera(%.4f, %s)", t, arg))
	if err != nil {
		return nil, err
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return nil, err
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(s, prefix) {
		return nil, errors.New("the page gave no picture")
	}
	return base64.StdEncoding.DecodeString(s[len(prefix):])
}

func (b *browser) waitReady(session string, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		v, err := b.eval(session, `!!(window.replayReady && window.replay && window.replay.ready())`)
		if err == nil && string(v) == "true" {
			return nil
		}
		select {
		case <-b.dead:
			return errors.New("the browser closed before the page was ready")
		case <-time.After(200 * time.Millisecond):
		}
	}
	return errors.New("the page did not become ready (does it load? try the same address in a browser)")
}

// frame draws the picture of a time and gives it as PNG bytes.
func (b *browser) frame(session string, t float64) ([]byte, error) {
	v, err := b.eval(session, fmt.Sprintf("window.replay.frame(%.4f)", t))
	if err != nil {
		return nil, err
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return nil, err
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(s, prefix) {
		return nil, errors.New("the page gave no picture")
	}
	return base64.StdEncoding.DecodeString(s[len(prefix):])
}

func (b *browser) close() {
	b.call("", "Browser.close", map[string]any{}) // a clean end
	b.to.Close()
	done := make(chan struct{})
	go func() { b.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		b.cmd.Process.Kill()
		<-done
	}
	os.RemoveAll(b.dir)
}

// cameraSync is the sync of the camera video in the page's object about it: the time of the video that matches the time 0 of the audio.
func cameraSync(raw json.RawMessage) float64 {
	var c struct {
		Sync float64 `json:"sync"`
	}
	json.Unmarshal(raw, &c)
	return c.Sync
}

// extractCameraFrames cuts the video into pictures, one for every frame of the render: file i (from 1) is the picture for the frame i-1+lead.
// Frames before the video begins are left out (lead says how many); a video that ends early just gives fewer files.
func extractCameraFrames(ctx context.Context, ffmpeg, video, dir string, sync, start float64, fps, frames, first int) error {
	lead := 0
	camStart := start + sync
	if camStart < 0 {
		lead = int(math.Ceil(-camStart*float64(fps) - 1e-9))
		camStart += float64(lead) / float64(fps)
	}
	args := []string{"-y", "-loglevel", "error"}
	if camStart > 0.0001 {
		args = append(args, "-ss", fmt.Sprintf("%.4f", camStart))
	}
	args = append(args, "-i", video, "-vf", fmt.Sprintf("fps=%d,scale='min(1920,iw)':'min(1920,ih)':force_original_aspect_ratio=decrease:flags=lanczos", fps),
		"-frames:v", fmt.Sprint(max(frames-lead, 1)), "-q:v", "3", "-start_number", fmt.Sprint(first+lead+1), filepath.Join(dir, "c%06d.jpg"))
	fmt.Printf("cutting the camera video into pictures (sync %+.3f s)...\n", sync)
	cmd := exec.CommandContext(ctx, ffmpeg, args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("the camera video: %w", err)
	}
	return nil
}
