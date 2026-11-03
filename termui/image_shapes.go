package termui

import (
	"math/bits"

	"github.com/moahbeatlab/404tui/handheld"
)

// Cell size in pixels, as the screens count them.
const (
	cellW, cellH = handheld.CellW, handheld.CellH
	npix         = cellW * cellH
)

// mask is the pixels of a cell that a character covers, a bit for each of the 8x16.
type mask [2]uint64

func (m *mask) set(x, y int) { i := y*cellW + x; m[i/64] |= 1 << (i % 64) }
func (m mask) get(x, y int) bool {
	i := y*cellW + x
	return m[i/64]>>(i%64)&1 == 1
}

func (m mask) xor(o mask) int { return bits.OnesCount64(m[0]^o[0]) + bits.OnesCount64(m[1]^o[1]) }

type shape struct {
	r    rune
	m    mask
	kind int // 0 plain, 1 eighths, 2 quadrants
}

var shapes []shape

func rectMask(x0, y0, x1, y1 int) (m mask) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			m.set(x, y)
		}
	}
	return
}

func orMask(a, b mask) mask { return mask{a[0] | b[0], a[1] | b[1]} }

func init() {
	add := func(r rune, m mask, kind int) { shapes = append(shapes, shape{r, m, kind}) }
	add(' ', mask{}, 0)
	add('█', rectMask(0, 0, 8, 16), 0)
	add('▀', rectMask(0, 0, 8, 8), 0)
	add('▄', rectMask(0, 8, 8, 16), 0)
	add('▌', rectMask(0, 0, 4, 16), 0)
	add('▐', rectMask(4, 0, 8, 16), 0)
	// lower bars in eighths of the height (2 px), left bars in eighths of the width (1 px)
	for i, r := range []rune("▁▂▃▅▆▇") {
		k := []int{1, 2, 3, 5, 6, 7}[i]
		add(r, rectMask(0, 16-2*k, 8, 16), 1)
	}
	for i, r := range []rune("▏▎▍▋▊▉") {
		k := []int{1, 2, 3, 5, 6, 7}[i]
		add(r, rectMask(0, 0, k, 16), 1)
	}
	add('▔', rectMask(0, 0, 8, 2), 1)
	add('▕', rectMask(7, 0, 8, 16), 1)
	// quarters: upper left, upper right, lower left, lower right
	ul, ur, ll, lr := rectMask(0, 0, 4, 8), rectMask(4, 0, 8, 8), rectMask(0, 8, 4, 16), rectMask(4, 8, 8, 16)
	add('▘', ul, 2)
	add('▝', ur, 2)
	add('▖', ll, 2)
	add('▗', lr, 2)
	add('▚', orMask(ul, lr), 2)
	add('▞', orMask(ur, ll), 2)
	add('▙', orMask(orMask(ul, ll), lr), 2)
	add('▛', orMask(orMask(ul, ur), ll), 2)
	add('▜', orMask(orMask(ul, ur), lr), 2)
	add('▟', orMask(orMask(ur, ll), lr), 2)
}
