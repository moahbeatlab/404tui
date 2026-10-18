//go:build !linux

package record

// StartRing has nothing to listen with off Linux: the recorder program is used.
func (b execBackend) StartRing(device string, seconds int) (*Ring, error) {
	return nil, ErrNoNative
}
