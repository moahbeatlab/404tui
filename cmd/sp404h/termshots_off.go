//go:build !termshots

package main

import (
	"errors"

	"github.com/moahbeatlab/404tui/core"
)

func termShot(*core.App, string) error {
	return errors.New("this build has no terminal pictures: build with -tags termshots")
}
