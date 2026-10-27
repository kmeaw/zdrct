//go:build windows
// +build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func openBrowser(url string, config *Config, execPath string, userDataDir string) (string, error) {
	socksProxy := ""
	if config != nil {
		socksProxy = strings.TrimSpace(config.SocksProxy)
	}
	if socksProxy == "" {
		return "", openDefaultBrowser(url)
	}

	if execPath == "" {
		var err error
		execPath, err = os.Executable()
		if err != nil {
			warning := supermiumFallbackMessage("", err)
			return warning, openDefaultBrowser(url)
		}
	}

	execDir := filepath.Dir(execPath)
	supermiumExe := filepath.Join(execDir, "Supermium", "chrome.exe")
	if _, err := os.Stat(supermiumExe); err != nil {
		warning := supermiumFallbackMessage(supermiumExe, err)
		return warning, openDefaultBrowser(url)
	}

	if userDataDir == "" {
		if config != nil {
			userDataDir = filepath.Join(config.zdrctConfigDir, "webview")
		} else {
			userDataDir = filepath.Join(os.TempDir(), "zdrct-webview")
		}
	}

	args := []string{
		"--disable-encryption",
		"--disable-machine-id",
		"--no-default-browser-check",
		"--no-first-run",
		"--user-data-dir=" + userDataDir,
		"--ungoogled-supermium",
		"--classic-omnibox",
		"--classic-omnibox-border",
		"--compact-ui",
		"--enable-features=SupermiumCustomTabs",
		"--disable-features=DownloadBubble,TabHoverCards,PowerBookmarksSidePanel",
		"--app",
		"--window-name=zdrct",
	}
	if socksProxy != "" {
		args = append(args, "--proxy-server=socks5://"+socksProxy)
	}
	args = append(args, url)

	if err := exec.Command(supermiumExe, args...).Start(); err != nil {
		warning := supermiumFallbackMessage(supermiumExe, err)
		return warning, openDefaultBrowser(url)
	}

	return "", nil
}

func openDefaultBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func supermiumFallbackMessage(supermiumExe string, reason error) string {
	location := "next to zdrct.exe"
	if supermiumExe != "" {
		location = fmt.Sprintf("at %q", supermiumExe)
	}

	return fmt.Sprintf(
		"SOCKS proxy is configured, but Supermium could not be used %s: %s\n\n"+
			"The default Windows browser was opened instead, so the OAuth browser window may not use the SOCKS proxy.\n\n"+
			"To proxy OAuth on Windows, download Supermium from https://github.com/win32ss/supermium/releases, extract the nonsetup ZIP, and place the Supermium folder next to zdrct.exe.",
		location,
		reason,
	)
}
