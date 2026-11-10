package generate

// Euclid spreads hits over steps as evenly as it can (Bjorklund's algorithm), then turns the result by rot steps: Euclid(3, 8, 0) is
// x..x..x., Euclid(5, 8, 0) is x.xx.xx.
func Euclid(hits, steps, rot int) []bool {
	if steps <= 0 {
		return nil
	}
	hits = max(0, min(hits, steps))
	out := make([]bool, steps)
	if hits == 0 {
		return out
	}
	// the counting form of Bjorklund: a hit falls where the running sum wraps
	for i := 0; i < steps; i++ {
		out[i] = (i*hits)%steps < hits
	}
	// that form puts a hit on step 0 and spreads the rest evenly; the classic notation is the same up to a turn
	r := ((rot % steps) + steps) % steps
	if r == 0 {
		return out
	}
	rotated := make([]bool, steps)
	for i := range out {
		rotated[(i+r)%steps] = out[i]
	}
	return rotated
}
