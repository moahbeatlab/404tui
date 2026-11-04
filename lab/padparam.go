package lab

import (
	"errors"
	"fmt"

	"github.com/moahbeatlab/404tui/proto"
)

// PadParam is a setting of a pad (the ids of proto.Params: level 0x69, balance 0x78, pitch 0x73, attack 0x7D ...) as the
// project file holds it.
func (l *Lab) PadParam(n int, id byte) (int32, error) {
	if l.pad == nil {
		return 0, errors.New("there is no project")
	}
	if n < 0 || n > 159 {
		return 0, fmt.Errorf("no pad %d", n)
	}
	blk, err := proto.ParsePadBlock(l.pad.Blocks[n])
	if err != nil {
		return 0, err
	}
	v, ok := blk.Param(id)
	if !ok {
		return 0, fmt.Errorf("parameter %#x is not in the project file", id)
	}
	return v, nil
}

// SetPadParam sets a setting of a pad, kept inside the range the unit allows for it.
func (l *Lab) SetPadParam(n int, id byte, v int32) error {
	if l.pad == nil {
		return errors.New("there is no project")
	}
	if n < 0 || n > 159 {
		return fmt.Errorf("no pad %d", n)
	}
	field, ok := proto.BlockField(id)
	if !ok {
		return fmt.Errorf("parameter %#x is not in the project file", id)
	}
	for _, p := range proto.Params {
		if p.ID == id && p.Scope == proto.ScopePad {
			v = min(max(v, p.Min), p.Max)
		}
	}
	blk, err := proto.ParsePadBlock(l.pad.Blocks[n])
	if err != nil {
		return err
	}
	blk.SetField(field, uint32(v))
	l.pad.Blocks[n] = blk.Raw()
	return nil
}

// SetupPad puts the settings a pad that was never set up has at zero, where the unit wants more, to their defaults: the pan at its
// centre, the hold at 100%, the time stretch at 100%, the sample's BPM at 120. It says how many it changed. A pad that already
// has sensible values is left as it is.
func (l *Lab) SetupPad(n int) (int, error) {
	if l.pad == nil {
		return 0, errors.New("there is no project")
	}
	if n < 0 || n > 159 {
		return 0, fmt.Errorf("no pad %d", n)
	}
	blk, err := proto.ParsePadBlock(l.pad.Blocks[n])
	if err != nil {
		return 0, err
	}
	fixed := 0
	for _, f := range []struct {
		id       byte
		lo, hi   int32
		def      int32
		hasField bool
	}{{0x78, 13, 115, 64, true}, {0x7E, 1, 100, 100, true}, {0x76, 5000, 15000, 10000, true}, {0x6F, 4000, 20000, 12000, true}} {
		v, ok := blk.Param(f.id)
		field, fok := proto.BlockField(f.id)
		if !ok || !fok || (v >= f.lo && v <= f.hi) {
			continue
		}
		blk.SetField(field, uint32(f.def))
		fixed++
	}
	if fixed > 0 {
		l.pad.Blocks[n] = blk.Raw()
	}
	return fixed, nil
}

// SetPadParamRaw sets a field without keeping it inside its range (for what other tools wrote, and for tests of SetupPad).
func (l *Lab) SetPadParamRaw(n int, id byte, v int32) error {
	if l.pad == nil || n < 0 || n > 159 {
		return errors.New("no such pad")
	}
	field, ok := proto.BlockField(id)
	if !ok {
		return fmt.Errorf("parameter %#x is not in the project file", id)
	}
	blk, err := proto.ParsePadBlock(l.pad.Blocks[n])
	if err != nil {
		return err
	}
	blk.SetField(field, uint32(v))
	l.pad.Blocks[n] = blk.Raw()
	return nil
}

// PadParams is every setting of a pad that the project file has, read with one parse of the pad's block.
func (l *Lab) PadParams(n int) map[byte]int32 {
	out := map[byte]int32{}
	if l.pad == nil || n < 0 || n > 159 {
		return out
	}
	blk, err := proto.ParsePadBlock(l.pad.Blocks[n])
	if err != nil {
		return out
	}
	for _, pi := range proto.Params {
		if pi.Scope != proto.ScopePad {
			continue
		}
		if v, ok := blk.Param(pi.ID); ok {
			out[pi.ID] = v
		}
	}
	return out
}
