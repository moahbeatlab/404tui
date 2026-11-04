package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/device"
)

// Link is the SP-404MKII on the other end of the USB cable (the OTG port of the handheld).
type Link interface {
	Name() string
	Projects() ([]string, error)
	// Send writes the project folder dir over project number n (0 based) of the unit.
	Send(n int, dir string, progress func(done, total int, name string)) error
	Close() error
}

// DeviceLink is a Link over a device session (the real unit on a serial port, or the simulation).
type DeviceLink struct{ Dev *device.Device }

func (l DeviceLink) Name() string                { return l.Dev.PortName() }
func (l DeviceLink) Projects() ([]string, error) { return l.Dev.ProjectNames() }
func (l DeviceLink) Close() error                { return l.Dev.Close() }

// restoreSource reads the folder from this computer and the free space from the unit.
type restoreSource struct {
	app.Local
	dev *device.Device
}

func (r restoreSource) FreeKb() (uint32, error) { return r.dev.FreeKb() }

func (l DeviceLink) Send(n int, dir string, progress func(done, total int, name string)) error {
	plan, err := app.PlanProjectRestore(restoreSource{dev: l.Dev}, uint8(n), dir, nil)
	if err != nil {
		return err
	}
	total := len(plan.Files)
	return app.RestoreProject(l.Dev, plan, func(i int, name string) { progress(i, total, name) })
}

// job is a transfer in the background.
type job struct {
	mu          sync.Mutex
	done, total int
	name        string
	err         error
	finished    bool
	project     int
}

func (j *job) state() (done, total int, name string, err error, finished bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.done, j.total, j.name, j.err, j.finished
}

// Tick runs what has to happen without a key: looking for the unit while the UNIT screen is in front.
func (a *App) Tick() {
	a.splashTick()
	if a.connCh != nil {
		select {
		case l := <-a.connCh:
			a.connBusy = false
			if l != nil {
				a.link = l
				a.connAt = a.clock()
			}
		default:
		}
	}
	a.autosaveTick()
	a.indexTick()
	a.freezeTick()
	a.jobTick()
	a.tapTick()
	a.liveTick()
	a.recordingTick()
	a.midiTick()
	a.levelTick()
	// the handheld looks for the unit while UNIT is in front; a computer looks all the time (slowly), for PROJECTS to show
	wait := 2 * time.Second
	if Terminal {
		wait = 4 * time.Second
	}
	if (!Terminal && a.scr != Unit) || a.link != nil || a.Connect == nil || a.clock().Sub(a.connAt) < wait {
		return
	}
	a.connAt = a.clock()
	if !Terminal { // the handheld waits for it on its UNIT screen
		if l, err := a.Connect(); err == nil {
			a.link = l
		}
		return
	}
	// a computer looks in the background: opening a port that is not the unit can take a while, and the screen must not wait for it
	if a.connCh == nil {
		a.connCh = make(chan Link, 1)
	}
	if a.connBusy {
		return
	}
	a.connBusy = true
	go func(connect func() (Link, error), ch chan Link) {
		l, err := connect()
		if err != nil {
			l = nil
		}
		ch <- l
	}(a.Connect, a.connCh)
}

func (a *App) unitKey(k Key) {
	if a.job != nil {
		if _, _, _, _, fin := a.job.state(); !fin {
			return
		}
	}
	if Terminal && !a.onlineNow() { // offline: the projects of a folder
		switch k {
		case A:
			a.offlineMenu()
			return
		case X:
			return
		}
	}
	up, down := 15, 1 // the handheld lists the projects, a computer shows them as tiles, four to a row
	if Terminal {
		up, down = 12, 4
		switch k {
		case Left:
			a.unitCur = (a.unitCur + 15) % 16
			a.confirm = false
		case Right:
			a.unitCur = (a.unitCur + 1) % 16
			a.confirm = false
		}
	}
	switch k {
	case Up:
		a.unitCur = (a.unitCur + up) % 16
		a.confirm = false
	case Down:
		a.unitCur = (a.unitCur + down) % 16
		a.confirm = false
	case B:
		if a.confirm {
			a.confirm = false
			return
		}
		a.job = nil
		if Terminal {
			a.go_(Pads)
			return
		}
		a.go_(Save)
	case A:
		if a.link == nil && !Terminal {
			a.connAt = time.Time{}
			a.Tick()
			return
		}
		if a.confirm {
			a.confirm = false
			a.send()
			return
		}
		a.unitMenu()
	case X: // look at the unit again
		if a.link != nil {
			a.link.Close()
			a.link = nil
			a.connAt = time.Time{}
		}
	}
}

// send writes the project to a temporary folder and sends that over the project of the unit.
func (a *App) send() {
	// the copy for the transfer goes next to the projects on the card: /tmp may be memory
	os.MkdirAll(a.OutDir, 0o755)
	tmp, err := os.MkdirTemp(a.OutDir, ".404tui-send")
	if err != nil {
		a.say(err.Error(), true)
		return
	}
	if _, err := a.exportTo(tmp); err != nil {
		os.RemoveAll(tmp)
		a.say(err.Error(), true)
		return
	}
	j := &job{project: a.unitCur}
	a.job = j
	link, n, dir := a.link, a.unitCur, filepath.Join(tmp, a.ProjectName())
	go func() {
		err := link.Send(n, dir, func(done, total int, name string) {
			j.mu.Lock()
			j.done, j.total, j.name = done, total, name
			j.mu.Unlock()
		})
		os.RemoveAll(tmp)
		j.mu.Lock()
		j.err, j.finished = err, true
		j.mu.Unlock()
	}()
}

func (a *App) drawUnit(g Gfx) {
	if Terminal {
		a.drawProjects(g)
		return
	}
	if a.link == nil {
		g.Text(2, 3, "NO UNIT", cAmber, true)
		g.Text(2, 6, "Plug the SP-404MKII into a USB port (the OTG port", cText, false)
		g.Text(2, 7, "of a handheld) with a cable.", cText, false)
		g.Text(2, 9, "Looking for it...   A looks now", cDim, false)
		g.Text(2, 12, "Without it, SAVE writes the project as a", cDim, false)
		g.Text(2, 13, "folder for the SD card.", cDim, false)
		return
	}
	g.Text(2, 1, "CONNECTED", cGood, true)
	g.Text(2+len("CONNECTED")+2, 1, fit(a.link.Name(), contentCols-20), cDim, false)
	names, err := a.unitNames()
	if err != nil {
		g.Text(2, 3, fit(err.Error(), contentCols-4), cBad, false)
		return
	}
	for i := 0; i < 16; i++ {
		y := 3 + i
		nm := ""
		if i < len(names) {
			nm = names[i]
		}
		fg := cText
		if i == a.unitCur {
			g.Highlight(1, y, contentCols-2, 1)
			fg = cWhite
		}
		g.Text(2, y, fmt.Sprintf("%2d  %s", i+1, fit(nm, 28)), fg, i == a.unitCur)
	}
}

// unitContext is the panel's part of the UNIT screen: what is being sent and how far it is (x, y, w in cells).
func (a *App) unitContext(g Gfx, x, y, w int) {
	if Terminal && !a.onlineNow() {
		g.Text(x, y, "OFFLINE", cAmber, true)
		g.Text(x, y+1, fit(short2(a.offlineDir(), w), w), cDim, false)
		g.Text(x, y+3, fmt.Sprintf("project %d", a.unitCur+1), cText, true)
		if dir, ok := a.offlineProject(a.unitCur); ok {
			g.Text(x, y+4, fit(filepath.Base(dir), w), cText, false)
			g.Text(x, y+5, "A  open it in the editor", cDim, false)
		} else {
			g.Text(x, y+4, "empty", cDim, false)
			g.Text(x, y+5, "A  write the project here", cDim, false)
		}
		if n, ok := a.lastOffline(); ok {
			g.Text(x, y+7, fit(fmt.Sprintf("last open offline: %d", n+1), w), cDim, false)
		}
		return
	}
	if a.link == nil {
		g.Text(x, y, "no unit", cDim, false)
		return
	}
	g.Text(x, y, "SENDING", cDim, false)
	g.Text(x, y+1, fit(a.ProjectName(), w), cAmber, true)
	g.Text(x, y+2, fmt.Sprintf("to unit project %d", a.unitCur+1), cText, false)
	switch {
	case a.job != nil:
		done, total, name, err, fin := a.job.state()
		if fin && err == nil {
			done = total
		}
		frac := 0.0
		if total > 0 {
			frac = float64(done) / float64(total)
		}
		g.Bar(x, y+4, w, frac, cAmber, cPanel2)
		g.Text(x, y+5, fit(name, w), cDim, false)
		switch {
		case fin && err != nil:
			g.Text(x, y+6, fit(err.Error(), w), cBad, true)
		case fin:
			g.Text(x, y+6, "DONE", cGood, true)
		default:
			g.Text(x, y+6, fmt.Sprintf("%d of %d files", done, total), cText, false)
		}
	case a.confirm:
		g.Fill(x, y+4, w, 5, cAmberD)
		g.Frame(x, y+4, w, 5, cAmber)
		g.Text(x+1, y+5, "REPLACES unit", cWhite, true)
		g.Text(x+1, y+6, fmt.Sprintf("project %d.", a.unitCur+1), cWhite, true)
		g.Text(x+1, y+7, "A again: send", cText, false)
	}
}

// drawProjectTiles is the 16 projects of the unit as a 4x4 grid under the line that says it is connected.
func (a *App) drawProjects(g Gfx) {
	var names []string
	if a.onlineNow() {
		g.Text(2, 1, "ONLINE", cGood, true)
		g.Text(2+len("ONLINE")+2, 1, fit(a.link.Name(), contentCols-20), cDim, false)
		var err error
		if names, err = a.unitNames(); err != nil {
			g.Text(2, 3, fit(err.Error(), contentCols-4), cBad, false)
			return
		}
	} else {
		g.Text(2, 1, "OFFLINE", cAmber, true)
		where := short2(a.offlineDir(), contentCols-40)
		note := "  no unit plugged in"
		if a.link != nil {
			note = "  a unit is plugged in: t goes online"
		}
		g.Text(2+len("OFFLINE")+2, 1, where, cText, false)
		g.Text(contentCols-1-len(note), 1, note, cDim, false)
		names = make([]string, 16)
		for i := range names {
			if dir, ok := a.offlineProject(i); ok {
				names[i] = filepath.Base(dir)
			}
		}
	}
	a.drawProjectTiles(g, names)
}

func (a *App) drawProjectTiles(g Gfx, names []string) {
	cols := contentCols - 2
	ph := max((Rows-5)/4, 3)
	for i := 0; i < 16; i++ {
		x := 1 + (i%4)*cols/4
		tw := 1 + (i%4+1)*cols/4 - x - 1
		y := 2 + (i/4)*ph
		th := ph - 1
		nm := ""
		if i < len(names) {
			nm = names[i]
		}
		bg := cPanel
		if nm != "" {
			bg = cPanel2
		}
		g.Fill(x, y, tw, th, bg)
		num := fmt.Sprint(i + 1)
		g.Text(x+tw-len(num)-1, y, num, cDim, false)
		if nm == "" {
			g.Text(x+2, y, "empty", cDim, false)
		} else {
			g.Text(x+2, y, fit(nm, tw-len(num)-4), cText, true)
		}
		if !a.onlineNow() && a.slot == i+1 && nm != "" {
			g.Text(x+2, y+1, "open now", cGood, false)
		}
		if i == a.unitCur {
			g.Cursor(x, y, tw, th)
		}
	}
}

// unitNames is the names of the unit's projects, asked of the unit at most every two seconds: a screen is drawn many times a second,
// and each question is a round trip over USB that the live changes have to share.
func (a *App) unitNames() ([]string, error) {
	if a.linkNames != nil && a.clock().Sub(a.linkNamesAt) < 2*time.Second {
		return a.linkNames, nil
	}
	names, err := a.link.Projects()
	if err != nil {
		return nil, err
	}
	a.linkNames, a.linkNamesAt = names, a.clock()
	return names, nil
}
