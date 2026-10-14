package app_test

import (
	"testing"

	"github.com/moahbeatlab/404tui/app"
	"github.com/moahbeatlab/404tui/formats"
)

func TestGuessRolesReadsTheSampleNames(t *testing.T) {
	// Names in the style the pad table shows (from the screenshot: 14_Kick, 13_CHat, ...).
	rows := []app.PadRow{
		{Pad: pad(t, "A1"), HasSample: true, Name: "14_Kick_90"},
		{Pad: pad(t, "A2"), HasSample: true, Name: "snare_tight"},
		{Pad: pad(t, "A3"), HasSample: true, Name: "13_CHat_01"},
		{Pad: pad(t, "A4"), HasSample: true, Name: "09_OHat_124"},
		{Pad: pad(t, "A5"), HasSample: true, Name: "12_Clap"},
		{Pad: pad(t, "A6"), HasSample: true, Name: "conga_hi"},
		{Pad: pad(t, "A7")}, // no sample: ignored
	}
	roles := app.GuessRoles(rows)
	want := map[formats.Lane]string{
		formats.LaneKick:      "A1",
		formats.LaneSnare:     "A2",
		formats.LaneClosedHat: "A3",
		formats.LaneOpenHat:   "A4",
		formats.LaneClap:      "A5",
		formats.LanePerc:      "A6",
	}
	for lane, name := range want {
		got, ok := roles[lane]
		if !ok {
			t.Errorf("%s was not guessed", lane.LaneName())
			continue
		}
		if got != pad(t, name) {
			t.Errorf("%s guessed %s, want %s", lane.LaneName(), got, name)
		}
	}
}

func TestGuessRolesGivesEachPadToOneLane(t *testing.T) {
	// A generic "hat" falls to the closed hat; it must not also be taken as the open hat.
	rows := []app.PadRow{
		{Pad: pad(t, "A1"), HasSample: true, Name: "Hat"},
		{Pad: pad(t, "A2"), HasSample: true, Name: "Kick"},
	}
	roles := app.GuessRoles(rows)
	if _, ok := roles[formats.LaneOpenHat]; ok {
		t.Error("a generic hat was taken as the open hat")
	}
	if got, ok := roles[formats.LaneClosedHat]; !ok || got != pad(t, "A1") {
		t.Errorf("closed hat = %v (ok %v), want A1", got, ok)
	}
}

func TestGuessRolesLeavesMissingLanesOut(t *testing.T) {
	rows := []app.PadRow{
		{Pad: pad(t, "A1"), HasSample: true, Name: "mystery blob"},
	}
	if roles := app.GuessRoles(rows); len(roles) != 0 {
		t.Errorf("guessed %d roles from an unnameable kit, want none", len(roles))
	}
}
