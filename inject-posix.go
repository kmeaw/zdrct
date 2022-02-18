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
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mattn/anko/env"
	"github.com/mattn/anko/vm"
	"github.com/yookoala/realpath"
)

func inject(exePath string, rconPassword string, script string, args ...string) error {
	e := env.NewEnv()
	e.Define("log", func(format string, args ...interface{}) {
		log.Printf(format, args...)
	})
	e.Define("chr", func(b byte) string {
		return string([]byte{b})
	})
	_, err := vm.Execute(e, nil, script)
	if err != nil {
		return fmt.Errorf("cannot compile user script: %w", err)
	}

	real, err := realpath.Realpath(exePath)
	if err != nil {
		real = exePath
	}

	var file string
	cmd := exec.Command(exePath, args...)
	cmd.Dir, file = filepath.Split(real)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()
	time.Sleep(time.Second * 2) // FIXME: wait process to initialize

	pid := cmd.Process.Pid
	patcher, err := NewPatcher(pid, file, rconPassword)
	if err != nil {
		syscall.PtraceCont(pid, 0)
		cmd.Process.Signal(syscall.SIGSTOP)
		syscall.Wait4(pid, nil, syscall.WNOHANG, nil)
		syscall.PtraceDetach(pid)
		cmd.Process.Signal(syscall.SIGCONT)
		cmd.Process.Kill()
		return fmt.Errorf("patcher error: %w", err)
	}
	e.Define("patcher", patcher)
	serr, err := vm.Execute(e, nil, "patch()")
	if err != nil {
		log.Printf("Cannot run patch script: %s", err)
		return err
	}
	if serr != nil {
		log.Printf("Error while executing patch script: %s", serr)
		if err, ok := serr.(error); ok {
			return err
		} else {
			return fmt.Errorf("%s", serr)
		}
	}

	go func() {
		// We do not use "wait" syscall to avoid accidentally stealing
		// events from the tracer.
		stat, err := os.Open(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			log.Printf("cannot open watch file: %s", err)
			return
		}
		defer stat.Close()
		buf := make([]byte, 4096)
		for {
			time.Sleep(time.Second)
			stat.Seek(0, os.SEEK_SET)
			_, err = stat.Read(buf)
			if err != nil {
				break
			}
		}

		log.Println("shutting down rcon server")
		patcher.Shutdown()
		patcher.RconServer.Stop()
		patcher.RconServer = nil
	}()

	return nil
}

func InitSound() error {
	return nil
}

func PlaySound(_ string) {
	// TODO: implement playsound for posix
}

// vim: ai:ts=8:sw=8:noet:syntax=go
