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
