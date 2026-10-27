// mklib makes the folder the offline clips run on: PROJECT_01 with a blank PADCONF.BIN, and the
// audio files given on the command line under SD/IMPORT. Run it through record.py, or by hand:
//
//	go run ./docs/video/mklib /tmp/404lib drum_break.wav
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/moahbeatlab/404tui/formats"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mklib <folder> [audio file ...]")
		os.Exit(2)
	}
	dir := os.Args[1]
	project := filepath.Join(dir, "PROJECT_01")
	imp := filepath.Join(dir, "SD", "IMPORT")
	for _, d := range []string{project, imp} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			fail(err)
		}
	}
	if err := os.WriteFile(filepath.Join(project, "PADCONF.BIN"), formats.NewPadconf(nil, "PROJECT_01").Bytes(), 0o644); err != nil {
		fail(err)
	}
	for _, src := range os.Args[2:] {
		if err := copyFile(src, filepath.Join(imp, filepath.Base(src))); err != nil {
			fail(err)
		}
	}
	fmt.Println("made", dir)
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mklib:", err)
	os.Exit(1)
}
