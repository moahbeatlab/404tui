package midiin

import (
	"fmt"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// On Windows the MIDI input is the system's own (winmm.dll), called without cgo, so every Windows
// build has it, ARM included. System exclusive messages are not taken (they need buffers handed to
// the driver, and nothing here acts on them).

var (
	winmm          = syscall.NewLazyDLL("winmm.dll")
	procNumDevs    = winmm.NewProc("midiInGetNumDevs")
	procDevCaps    = winmm.NewProc("midiInGetDevCapsW")
	procOpen       = winmm.NewProc("midiInOpen")
	procStart      = winmm.NewProc("midiInStart")
	procStop       = winmm.NewProc("midiInStop")
	procReset      = winmm.NewProc("midiInReset")
	procClose      = winmm.NewProc("midiInClose")
	procOutNum     = winmm.NewProc("midiOutGetNumDevs")
	procOutCaps    = winmm.NewProc("midiOutGetDevCapsW")
	procOutOpen    = winmm.NewProc("midiOutOpen")
	procOutShort   = winmm.NewProc("midiOutShortMsg")
	procOutClose   = winmm.NewProc("midiOutClose")
	callbackOnce   sync.Once
	callbackFunc   uintptr
	listenersMu    sync.Mutex
	listeners      = map[uintptr]func(Event){}
	nextListenerID uintptr
)

const (
	callbackFunction = 0x30000 // CALLBACK_FUNCTION
	mimData          = 0x3C3   // MIM_DATA: a short message
)

type midiInCaps struct {
	mid, pid      uint16
	driverVersion uint32
	name          [32]uint16
	support       uint32
}

// Default is the MIDI input of this build: the system's.
func Default() Source { return winSource{} }

type winSource struct{}

// inputs names the inputs there are, by their device number.
func inputs() []string {
	n, _, _ := procNumDevs.Call()
	var names []string
	for i := uintptr(0); i < n; i++ {
		var caps midiInCaps
		if r, _, _ := procDevCaps.Call(i, uintptr(unsafe.Pointer(&caps)), unsafe.Sizeof(caps)); r != 0 {
			names = append(names, fmt.Sprintf("input %d", i))
			continue
		}
		names = append(names, syscall.UTF16ToString(caps.name[:]))
	}
	return names
}

func (winSource) Ports() ([]string, error) { return inputs(), nil }

// dispatch is the one callback every open input uses: Windows calls it on a thread of its own
// for each message, with the instance number given when the input was opened.
func dispatch(_, msg, instance, param1, _ uintptr) uintptr {
	if msg != mimData {
		return 0
	}
	listenersMu.Lock()
	on := listeners[instance]
	listenersMu.Unlock()
	if on == nil {
		return 0
	}
	b := []byte{byte(param1), byte(param1 >> 8), byte(param1 >> 16)}
	switch {
	case b[0] >= 0xF8:
		b = b[:1]
	case b[0]&0xF0 == 0xC0 || b[0]&0xF0 == 0xD0:
		b = b[:2]
	}
	if e, ok := Parse(b); ok {
		on(e)
	}
	return 0
}

func (winSource) Open(part string, on func(Event)) (func(), string, error) {
	names := inputs()
	id := -1
	for i, n := range names {
		if strings.Contains(strings.ToLower(n), strings.ToLower(part)) {
			id = i
			break
		}
	}
	if id < 0 {
		return nil, "", fmt.Errorf("no MIDI input with %q in its name (there are: %s)", part, strings.Join(names, ", "))
	}
	callbackOnce.Do(func() { callbackFunc = syscall.NewCallback(dispatch) })
	listenersMu.Lock()
	nextListenerID++
	instance := nextListenerID
	listeners[instance] = on
	listenersMu.Unlock()
	forget := func() {
		listenersMu.Lock()
		delete(listeners, instance)
		listenersMu.Unlock()
	}
	var handle uintptr
	if r, _, _ := procOpen.Call(uintptr(unsafe.Pointer(&handle)), uintptr(id), callbackFunc, instance, callbackFunction); r != 0 {
		forget()
		return nil, "", fmt.Errorf("opening the MIDI input %q failed (winmm error %d; is another program using it?)", names[id], r)
	}
	if r, _, _ := procStart.Call(handle); r != 0 {
		procClose.Call(handle)
		forget()
		return nil, "", fmt.Errorf("starting the MIDI input %q failed (winmm error %d)", names[id], r)
	}
	return func() {
		procStop.Call(handle)
		procReset.Call(handle)
		procClose.Call(handle)
		forget()
	}, names[id], nil
}

// outputs names the outputs there are, by their device number. A MIDIOUTCAPSW starts like an input's.
func outputs() []string {
	n, _, _ := procOutNum.Call()
	var names []string
	for i := uintptr(0); i < n; i++ {
		var caps midiOutCaps
		if r, _, _ := procOutCaps.Call(i, uintptr(unsafe.Pointer(&caps)), unsafe.Sizeof(caps)); r != 0 {
			names = append(names, fmt.Sprintf("output %d", i))
			continue
		}
		names = append(names, syscall.UTF16ToString(caps.name[:]))
	}
	return names
}

type midiOutCaps struct {
	mid, pid                  uint16
	driverVersion             uint32
	name                      [32]uint16
	technology, voices, notes uint16
	channelMask, support      uint32
}

type winOut struct{ handle uintptr }

func (o *winOut) Send(msg []byte) error {
	var packed uintptr
	for i, b := range msg {
		if i < 3 {
			packed |= uintptr(b) << (8 * i)
		}
	}
	if r, _, _ := procOutShort.Call(o.handle, packed); r != 0 {
		return fmt.Errorf("sending MIDI failed (winmm error %d)", r)
	}
	return nil
}

func (o *winOut) Close() { procOutClose.Call(o.handle) }

// OpenOut opens the first output with part in its name.
func (winSource) OpenOut(part string) (Out, string, error) {
	names := outputs()
	for i, n := range names {
		if !strings.Contains(strings.ToLower(n), strings.ToLower(part)) {
			continue
		}
		var handle uintptr
		if r, _, _ := procOutOpen.Call(uintptr(unsafe.Pointer(&handle)), uintptr(i), 0, 0, 0); r != 0 {
			return nil, "", fmt.Errorf("opening the MIDI output %q failed (winmm error %d; is another program using it?)", n, r)
		}
		return &winOut{handle: handle}, n, nil
	}
	return nil, "", fmt.Errorf("no MIDI output with %q in its name (there are: %s)", part, strings.Join(names, ", "))
}
