package handheld

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
	a.autosaveTick()
	a.indexTick()
	a.freezeTick()
	if a.scr != Unit || a.link != nil || a.Connect == nil || a.clock().Sub(a.connAt) < 2*time.Second {
		return
	}
	a.connAt = a.clock()
	if l, err := a.Connect(); err == nil {
		a.link = l
	}
}

func (a *App) unitKey(k Key) {
	if a.job != nil {
		if _, _, _, _, fin := a.job.state(); !fin {
			return
		}
	}
	switch k {
	case Up:
		a.unitCur = (a.unitCur + 15) % 16
		a.confirm = false
	case Down:
		a.unitCur = (a.unitCur + 1) % 16
		a.confirm = false
	case B:
		if a.confirm {
			a.confirm = false
			return
		}
		a.job = nil
		a.go_(Save)
	case A:
		if a.link == nil {
			a.connAt = time.Time{}
			a.Tick()
			return
		}
		if !a.confirm {
			a.confirm = true
			return
		}
		a.confirm = false
		a.send()
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
	names, err := a.link.Projects()
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
