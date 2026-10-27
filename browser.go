//go:build !windows
// +build !windows

package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

func openBrowser(url string, config *Config, execPath string, userDataDir string) (string, error) {
	switch runtime.GOOS {
	case "linux":
		return "", exec.Command("xdg-open", url).Start()
	case "darwin":
		return "", exec.Command("open", url).Start()
	}

	return "", fmt.Errorf("opening a browser is not supported on %s", runtime.GOOS)
}
