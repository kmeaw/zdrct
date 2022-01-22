//go:build !windows
// +build !windows

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

func InitSound() error {
	return nil
}

func PlaySound(_ string) {
	// TODO: implement playsound for posix
}

// vim: ai:ts=8:sw=8:noet:syntax=go
