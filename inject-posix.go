//go:build !windows
// +build !windows

package main

import (
	"os"
	"os/exec"
	"path"
)

func inject(exePath string, args ...string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	dir, file := filepath.Split(exePath)
	if dir != "" {
		err := os.Chdir(dir)
		if err != nil {
			return err
		}
	}

	cmd := exec.Command(exePath)
	cmd.Env = append(os.Environ(), "LD_PRELOAD="+filepath.Join(wd, "libinjector.so"))
	return cmd.Run()
}

func InitSound() error {
	return nil
}

func PlaySound() {
	// TODO: implement playsound for posix
}

// vim: ai:ts=8:sw=8:noet:syntax=go
