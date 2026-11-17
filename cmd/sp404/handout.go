package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/moahbeatlab/404tui/handout"
)

const handoutUsage = `usage: sp404 handout <project folder or zip> [options]

Writes the project as an EPUB for an e-reader: every bank at a glance (key, tempo, pads, patterns), then every pattern as step grids, a
bar to a grid. The project is only read.

  -o file.epub     where the book goes (default: <project>.epub beside the project)
  -notes file      keys, names and lines of your own, as JSON (default: handout.json in the project folder, if it is there):
                   {"title": "Tech House Live", "keys": {"A": "Am", "A9": "A"}, "banks": {"A": "Drums and bass"}, "patterns": {"A1": "Groove"}}
`

func runHandout(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(handoutUsage)
		return nil
	}
	src := filepath.Clean(args[0])
	fl := flag.NewFlagSet("handout", flag.ContinueOnError)
	out := fl.String("o", "", "")
	notesPath := fl.String("notes", "", "")
	if err := fl.Parse(args[1:]); err != nil {
		return err
	}
	l, err := loadProject(src)
	if err != nil {
		return err
	}
	p, err := handout.FromLab(l)
	if err != nil {
		return err
	}
	np := *notesPath
	if np == "" {
		np = filepath.Join(src, "handout.json")
	}
	if p.Notes, err = handout.ReadNotes(np); err != nil {
		return err
	}
	b, err := handout.Build(p, time.Now())
	if err != nil {
		return err
	}
	dst := *out
	if dst == "" {
		dst = strings.TrimSuffix(src, filepath.Ext(src)) + ".epub"
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return err
	}
	fmt.Printf("written: %s (%d patterns, %.1f MB)\n", dst, len(p.Patterns), float64(len(b))/1e6)
	return nil
}
