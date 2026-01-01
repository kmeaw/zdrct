// Copyright 2022 The age Authors. All rights reserved.

package main

import (
	"log"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

func init() {
	// Some instances of the Windows Console (e.g., cmd.exe and Windows PowerShell)
	// do not have the virtual terminal processing enabled, which is necessary to
	// make terminal escape sequences work. For this reason the clearLine function
	// may not properly work. Here we enable the virtual terminal processing, if
	// possible.
	//
	// See https://learn.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences.

	const (
		ENABLE_PROCESSED_OUTPUT            uint32 = 0x1
		ENABLE_VIRTUAL_TERMINAL_PROCESSING uint32 = 0x4
	)

	kernel32DLL := windows.NewLazySystemDLL("Kernel32.dll")
	setConsoleMode := kernel32DLL.NewProc("SetConsoleMode")

	var mode uint32
	if err := syscall.GetConsoleMode(syscall.Handle(os.Stdout.Fd()), &mode); err != nil {
		log.Printf("GetConsole mode failed: %s", err)
		return
	}

	mode |= ENABLE_PROCESSED_OUTPUT
	mode |= ENABLE_VIRTUAL_TERMINAL_PROCESSING

	// If the SetConsoleMode function fails, the return value is zero.
	// See https://learn.microsoft.com/en-us/windows/console/setconsolemode#return-value.
	if ret, _, _ := setConsoleMode.Call(os.Stdout.Fd(), uintptr(mode)); ret == 0 {
		log.Println("SetConsoleMode failed")
		return
	}
}

// https://github.com/FiloSottile/age
//
// Copyright 2019 The age Authors
// Copyright 2019 Google LLC
// Copyright 2022 Filippo Valsorda
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
//    * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
//    * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
//    * Neither the name of the age project nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
