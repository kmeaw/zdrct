//go:build windows
// +build windows

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
	"bytes"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/mattn/anko/env"
	"github.com/mattn/anko/vm"
	"golang.org/x/sys/windows"
)

var ERROR_OKAY syscall.Errno = 0

func S(input string) *uint16 {
	u, err := syscall.UTF16FromString(input)
	if err != nil {
		panic(err)
	}

	return &u[0]
}

var sounds map[string]string
var mciSendString *windows.LazyProc
var mciGetErrorString *windows.LazyProc

func InitSound() error {
	winmm := windows.NewLazySystemDLL("winmm.dll")
	err := winmm.Load()
	if err != nil {
		return err
	}

	mciSendString = winmm.NewProc("mciSendStringW")
	err = mciSendString.Find()
	if err != nil {
		return err
	}

	mciGetErrorString = winmm.NewProc("mciGetErrorStringA")
	err = mciGetErrorString.Find()
	if err != nil {
		return fmt.Errorf("cannot resolve mciGetErrorString: %w", err)
	}

	sounds = make(map[string]string)

	return nil
}

func PlaySound(filename string) error {
	// TODO: protect sounds
	name := sounds[filename]
	if name == "" {
		name = base64.RawURLEncoding.EncodeToString([]byte(filename))
		sounds[filename] = name
		mciSendString.Call(
			uintptr(unsafe.Pointer(S("open assets\\"+filename+" type mpegvideo alias "+name))),
			0,
			0,
			0,
		)
	}
	ret, _, _ := mciSendString.Call(
		uintptr(unsafe.Pointer(S("play "+name))),
		0,
		0,
		0,
	)
	if ret != 0 {
		errbuf := make([]byte, 4096)
		mciGetErrorString.Call(
			uintptr(ret),
			uintptr(unsafe.Pointer(&errbuf[0])),
			uintptr(len(errbuf)),
		)
		idx := bytes.IndexByte(errbuf, 0)
		if idx != -1 {
			errbuf = errbuf[:idx]
		}
		return fmt.Errorf("mciSendString failed: %s", errbuf)
	}

	return nil
}

func patch_russian_doom(patcher *Patcher) error {
	you_got_it := patcher.ScanString("YOU GOT IT")
	load_language_string := you_got_it.StoreRef()
	cheat_func3 := load_language_string.LoadRef()
	cheat_func3 = cheat_func3.FuncAlign()
	p_GiveArtifact, err := cheat_func3.ArgRef(2, patcher.Nil()).Result()
	if err != nil {
		return err
	}

	a_secret_is_revealed := patcher.ScanString("A SECRET IS REVEALED")
	load_language_string2 := a_secret_is_revealed.StoreRef()
	sector9_handler := load_language_string2.LoadRef()
	console_player, err := sector9_handler.MulAdd().Result()
	if err != nil {
		return err
	}

	log.Printf("console_player: %x", console_player)
	log.Printf("P_GiveArtifact: %x", p_GiveArtifact)

	return nil
}

const (
	EXCEPTION_DEBUG_EVENT      = 1
	CREATE_THREAD_DEBUG_EVENT  = 2
	CREATE_PROCESS_DEBUG_EVENT = 3
	EXIT_THREAD_DEBUG_EVENT    = 4
	EXIT_PROCESS_DEBUG_EVENT   = 5
	LOAD_DLL_DEBUG_EVENT       = 6
	UNLOAD_DLL_DEBUG_EVENT     = 7
	OUTPUT_DEBUG_STRING_EVENT  = 8
	RIP_EVENT                  = 9
)

const (
	DBG_CONTINUE = 0x00010002
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

	dir, file := filepath.Split(exePath)
	if dir != "" {
		err := os.Chdir(dir)
		if err != nil {
			return fmt.Errorf("chdir failed: %w", err)
		}
	}

	var si windows.StartupInfo
	var pi windows.ProcessInformation
	err = windows.CreateProcess(
		nil,
		S(strings.Join(append([]string{file}, args...), " ")),
		nil, nil, false,
		windows.NORMAL_PRIORITY_CLASS|
			windows.CREATE_NEW_CONSOLE|
			windows.CREATE_NEW_PROCESS_GROUP|
			windows.CREATE_SUSPENDED,
		nil,
		nil,
		&si,
		&pi,
	)
	if err != nil {
		return fmt.Errorf("CreateProcess has failed: %w", err)
	}

	log.Printf("New process with %d has been created.", pi.ProcessId)

	defer func() {
		if pi.Process != 0 {
			log.Printf("Terminating process %d", pi.ProcessId)
			windows.TerminateProcess(pi.Process, 1)
		}
	}()

	log.Println("Creating a patcher and running user script...")

	patcher, err := NewPatcher(pi.Process, pi.Thread, file, rconPassword)
	if err != nil {
		return fmt.Errorf("cannot create a patcher: %s", err)
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

	_, err = windows.ResumeThread(pi.Thread)
	if err != nil {
		return fmt.Errorf("cannot resume thread: %w", err)
	}

	go func(hProcess windows.Handle) {
		_, err := windows.WaitForSingleObject(hProcess, windows.INFINITE)
		if err != nil && err != ERROR_OKAY {
			log.Printf("cannot wait for process to finish: %s", err)
		}

		patcher.RconServer.Stop()
		patcher.RconServer = nil
	}(pi.Process)

	pi.Process = 0

	return nil
}

// vim: ai:ts=8:sw=8:noet:syntax=go
