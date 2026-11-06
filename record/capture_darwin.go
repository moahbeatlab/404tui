package record

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Native capture on a Mac: an AudioQueue on the input device, called through purego (no C compiler; the same way the playback library
// calls AudioToolbox). The device is found by name with the HAL (CoreAudio), the queue is pointed at it by its UID, and the sound comes
// as 32 bit float stereo at 48 kHz in buffers of 20 ms.

func init() { nativeOpener = openCoreAudio }

const (
	kAudioFormatLinearPCM       = 0x6C70636D // 'lpcm'
	kAudioFormatFlagIsFloat     = 1 << 0
	kAudioFormatFlagIsPacked    = 1 << 3
	kAudioObjectSystemObject    = 1
	kAudioHardwarePropertyDevs  = 0x64657623 // 'dev#'
	kAudioObjectPropertyScopeG  = 0x676C6F62 // 'glob'
	kAudioObjectPropertyScopeIn = 0x696E7074 // 'inpt'
	kAudioObjectPropertyName    = 0x6C6E616D // 'lnam'
	kAudioDevicePropertyUID     = 0x75696420 // 'uid '
	kAudioDevicePropertyStreams = 0x736C6179 // 'slay' (the stream configuration)
	kAudioQueuePropertyDevice   = 0x61716364 // 'aqcd'
	kCFStringEncodingUTF8       = 0x08000100
	captureBuffers              = 4
	captureFrames               = 960 // 20 ms
)

type audioStreamBasicDescription struct {
	mSampleRate       float64
	mFormatID         uint32
	mFormatFlags      uint32
	mBytesPerPacket   uint32
	mFramesPerPacket  uint32
	mBytesPerFrame    uint32
	mChannelsPerFrame uint32
	mBitsPerChannel   uint32
	mReserved         uint32
}

type audioQueueBuffer struct {
	mAudioDataBytesCapacity uint32
	mAudioData              uintptr
	mAudioDataByteSize      uint32
	mUserData               uintptr

	mPacketDescriptionCapacity uint32
	mPacketDescriptions        uintptr
	mPacketDescriptionCount    uint32
}

type audioObjectPropertyAddress struct{ mSelector, mScope, mElement uint32 }

var (
	apiOnce sync.Once
	apiErr  error

	audioQueueNewInput       func(format *audioStreamBasicDescription, callback uintptr, userData uintptr, runLoop, runLoopMode uintptr, flags uint32, out *uintptr) int32
	audioQueueAllocateBuffer func(q uintptr, size uint32, out **audioQueueBuffer) int32
	audioQueueEnqueueBuffer  func(q uintptr, buf *audioQueueBuffer, numDescs uint32, descs uintptr) int32
	audioQueueStart          func(q uintptr, startTime uintptr) int32
	audioQueueStop           func(q uintptr, immediate bool) int32
	audioQueueDispose        func(q uintptr, immediate bool) int32
	audioQueueSetProperty    func(q uintptr, id uint32, data unsafe.Pointer, size uint32) int32

	audioObjectGetPropertyDataSize func(object uint32, addr *audioObjectPropertyAddress, qSize uint32, q unsafe.Pointer, out *uint32) int32
	audioObjectGetPropertyData     func(object uint32, addr *audioObjectPropertyAddress, qSize uint32, q unsafe.Pointer, ioSize *uint32, out unsafe.Pointer) int32

	cfStringGetCString func(s uintptr, buf *byte, size int64, encoding uint32) bool
	cfRelease          func(o uintptr)
)

func loadAPI() error {
	apiOnce.Do(func() {
		toolbox, err := purego.Dlopen("/System/Library/Frameworks/AudioToolbox.framework/AudioToolbox", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			apiErr = err
			return
		}
		core, err := purego.Dlopen("/System/Library/Frameworks/CoreAudio.framework/CoreAudio", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			apiErr = err
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			apiErr = err
			return
		}
		purego.RegisterLibFunc(&audioQueueNewInput, toolbox, "AudioQueueNewInput")
		purego.RegisterLibFunc(&audioQueueAllocateBuffer, toolbox, "AudioQueueAllocateBuffer")
		purego.RegisterLibFunc(&audioQueueEnqueueBuffer, toolbox, "AudioQueueEnqueueBuffer")
		purego.RegisterLibFunc(&audioQueueStart, toolbox, "AudioQueueStart")
		purego.RegisterLibFunc(&audioQueueStop, toolbox, "AudioQueueStop")
		purego.RegisterLibFunc(&audioQueueDispose, toolbox, "AudioQueueDispose")
		purego.RegisterLibFunc(&audioQueueSetProperty, toolbox, "AudioQueueSetProperty")
		purego.RegisterLibFunc(&audioObjectGetPropertyDataSize, core, "AudioObjectGetPropertyDataSize")
		purego.RegisterLibFunc(&audioObjectGetPropertyData, core, "AudioObjectGetPropertyData")
		purego.RegisterLibFunc(&cfStringGetCString, cf, "CFStringGetCString")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
	})
	return apiErr
}

// cfString is a Go string from a CFString (the object is not released).
func cfString(ref uintptr) string {
	buf := make([]byte, 512)
	if ref == 0 || !cfStringGetCString(ref, &buf[0], int64(len(buf)), kCFStringEncodingUTF8) {
		return ""
	}
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

// stringProperty reads a property of an audio object that is a CFString (its name, its UID); the CFString is returned, to be kept
// (the queue is given the UID) and released by the caller.
func stringProperty(object, selector, scope uint32) (uintptr, bool) {
	addr := audioObjectPropertyAddress{selector, scope, 0}
	var ref uintptr
	size := uint32(unsafe.Sizeof(ref))
	if audioObjectGetPropertyData(object, &addr, 0, nil, &size, unsafe.Pointer(&ref)) != 0 || ref == 0 {
		return 0, false
	}
	return ref, true
}

// inputChannels is how many channels the device has on its input side.
func inputChannels(device uint32) int {
	addr := audioObjectPropertyAddress{kAudioDevicePropertyStreams, kAudioObjectPropertyScopeIn, 0}
	var size uint32
	if audioObjectGetPropertyDataSize(device, &addr, 0, nil, &size) != 0 || size < 8 {
		return 0
	}
	buf := make([]byte, size)
	if audioObjectGetPropertyData(device, &addr, 0, nil, &size, unsafe.Pointer(&buf[0])) != 0 {
		return 0
	}
	n := int(*(*uint32)(unsafe.Pointer(&buf[0]))) // AudioBufferList: the count, then (channels, bytes, data) a buffer, from byte 8
	ch := 0
	for i := 0; i < n && 8+16*i+4 <= len(buf); i++ {
		ch += int(*(*uint32)(unsafe.Pointer(&buf[8+16*i])))
	}
	return ch
}

// findInput is the UID (a CFString, to be released) and the name of the first input device whose name holds device.
func findInput(device string) (uid uintptr, name string, err error) {
	addr := audioObjectPropertyAddress{kAudioHardwarePropertyDevs, kAudioObjectPropertyScopeG, 0}
	var size uint32
	if st := audioObjectGetPropertyDataSize(kAudioObjectSystemObject, &addr, 0, nil, &size); st != 0 {
		return 0, "", fmt.Errorf("CoreAudio would not list the devices (%d)", st)
	}
	ids := make([]uint32, size/4)
	if len(ids) == 0 {
		return 0, "", ErrNoDevice
	}
	if st := audioObjectGetPropertyData(kAudioObjectSystemObject, &addr, 0, nil, &size, unsafe.Pointer(&ids[0])); st != 0 {
		return 0, "", fmt.Errorf("CoreAudio would not list the devices (%d)", st)
	}
	want := strings.ToLower(device)
	for _, id := range ids {
		nameRef, ok := stringProperty(id, kAudioObjectPropertyName, kAudioObjectPropertyScopeG)
		if !ok {
			continue
		}
		n := cfString(nameRef)
		cfRelease(nameRef)
		if !strings.Contains(strings.ToLower(n), want) || inputChannels(id) == 0 {
			continue
		}
		if ref, ok := stringProperty(id, kAudioDevicePropertyUID, kAudioObjectPropertyScopeG); ok {
			return ref, n, nil
		}
	}
	return 0, "", fmt.Errorf("%w: %q (is the unit plugged in and on?)", ErrNoDevice, device)
}

// the queues that are running, by the number their callback gets as its user data: a callback from the audio thread cannot carry a Go
// pointer.
var (
	queuesMu sync.Mutex
	queues   = map[uintptr]*captureQueue{}
	nextID   uintptr
	callback uintptr
)

type captureQueue struct {
	q        uintptr
	onFrames func([]float32)
	onEnd    func(error)
	stopped  bool
	floats   []float32
}

func inputCallback(userData uintptr, q uintptr, buf *audioQueueBuffer, startTime uintptr, numPackets uint32, descs uintptr) {
	queuesMu.Lock()
	c := queues[userData]
	queuesMu.Unlock()
	if c == nil || c.stopped {
		return
	}
	n := int(buf.mAudioDataByteSize) / 4
	if n > 0 && buf.mAudioData != 0 {
		data := *(*unsafe.Pointer)(unsafe.Pointer(&buf.mAudioData)) // the address the audio system gave, as a pointer (not a conversion of a number)
		src := unsafe.Slice((*float32)(data), n)
		c.floats = append(c.floats[:0], src...)
		c.onFrames(c.floats)
	}
	audioQueueEnqueueBuffer(q, buf, 0, 0)
}

// openCoreAudio listens to the input device whose name holds device.
func openCoreAudio(device string, onFrames func([]float32), onEnd func(error)) (io.Closer, error) {
	if err := loadAPI(); err != nil {
		return nil, fmt.Errorf("CoreAudio: %w", err)
	}
	uid, name, err := findInput(device)
	if err != nil {
		return nil, err
	}
	defer cfRelease(uid)
	queuesMu.Lock()
	if callback == 0 {
		callback = purego.NewCallback(inputCallback)
	}
	nextID++
	id := nextID
	c := &captureQueue{onFrames: onFrames, onEnd: onEnd}
	queues[id] = c
	queuesMu.Unlock()
	drop := func() { queuesMu.Lock(); delete(queues, id); queuesMu.Unlock() }
	desc := audioStreamBasicDescription{
		mSampleRate: 48000, mFormatID: kAudioFormatLinearPCM, mFormatFlags: kAudioFormatFlagIsFloat | kAudioFormatFlagIsPacked,
		mBytesPerPacket: 8, mFramesPerPacket: 1, mBytesPerFrame: 8, mChannelsPerFrame: 2, mBitsPerChannel: 32,
	}
	if st := audioQueueNewInput(&desc, callback, id, 0, 0, 0, &c.q); st != 0 {
		drop()
		return nil, fmt.Errorf("CoreAudio: the input queue would not open (%d)", st)
	}
	// point the queue at the device: the property takes its UID
	if st := audioQueueSetProperty(c.q, kAudioQueuePropertyDevice, unsafe.Pointer(&uid), uint32(unsafe.Sizeof(uid))); st != 0 {
		audioQueueDispose(c.q, true)
		drop()
		return nil, fmt.Errorf("CoreAudio: %q would not be the input (%d)", name, st)
	}
	for i := 0; i < captureBuffers; i++ {
		var buf *audioQueueBuffer
		if st := audioQueueAllocateBuffer(c.q, captureFrames*8, &buf); st != 0 {
			audioQueueDispose(c.q, true)
			drop()
			return nil, fmt.Errorf("CoreAudio: no buffer for the input (%d)", st)
		}
		audioQueueEnqueueBuffer(c.q, buf, 0, 0)
	}
	if st := audioQueueStart(c.q, 0); st != 0 {
		audioQueueDispose(c.q, true)
		drop()
		return nil, fmt.Errorf("CoreAudio: the input would not start (%d): is the program allowed to use the microphone (System Settings, Privacy)?", st)
	}
	return closerFunc(func() error {
		c.stopped = true
		audioQueueStop(c.q, true)
		audioQueueDispose(c.q, true)
		drop()
		return nil
	}), nil
}
