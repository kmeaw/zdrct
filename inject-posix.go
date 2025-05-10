//go:build !windows
// +build !windows

/**
 * Copyright 2022 kmeaw
 *
 * Licensed under the GNU Affero General Public License (AGPL).
 *
 * This program is free software: you can redistribute it and/or modify it
 * under the terms of the GNU Affero General Public License as published by the
 * Free Software Foundation, version 3 of the License.
 *
 * This program is distributed in the hope that it will be useful, but WITHOUT
 * ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
 * FITNESS FOR A PARTICULAR PURPOSE.  See the GNU Affero General Public License
 * for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/yookoala/realpath"
)

func inject(exePath string, args ...string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	real, err := realpath.Realpath(exePath)
	if err != nil {
		real = exePath
	}

	cmd := exec.Command(exePath, args...)
	cmd.Dir, _ = filepath.Split(real)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "LD_PRELOAD="+filepath.Join(wd, "libinjector.so"))

	ch := make(chan error, 1)
	go func() {
		ch <- cmd.Run()
	}()
	select {
	case <-time.After(2 * time.Second):
		return nil
	case err := <-ch:
		return err
	}
}

// vim: ai:ts=8:sw=8:noet:syntax=go
