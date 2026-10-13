package app

import (
	"slices"
	"strings"
	"testing"
)

func TestNaturalOrderCountsNumbersAsNumbers(t *testing.T) {
	names := []string{"clap_10.wav", "clap_2.wav", "clap_1.wav", "clap_20.wav", "clap_3.wav", "Clap_11.wav", "full_1.wav", "clap_02.wav"}
	slices.SortStableFunc(names, func(a, b string) int {
		la, lb := strings.ToLower(a), strings.ToLower(b)
		switch {
		case NaturalLess(la, lb):
			return -1
		case NaturalLess(lb, la):
			return 1
		}
		return 0
	})
	want := []string{"clap_1.wav", "clap_2.wav", "clap_02.wav", "clap_3.wav", "clap_10.wav", "Clap_11.wav", "clap_20.wav", "full_1.wav"}
	if !slices.Equal(names, want) {
		t.Fatalf("got %v, want %v", names, want)
	}
}
