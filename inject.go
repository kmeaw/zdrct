//go:build windows
// +build windows

package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"log"
	"syscall"
	"unsafe"

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

var shellcode = []byte{
	0x31, 0xC9, 0x64, 0x8B, 0x41, 0x30, // Find PEB
	0x8B, 0x40, 0x0C, 0x8B, 0x70, 0x14,
	0xAD, 0x96, 0xAD, 0x8B, 0x58, 0x10, 0x8B, 0x53, 0x3C, 0x01, 0xDA, 0x8B,
	0x52, 0x78, 0x01, 0xDA, 0x8B, 0x72, 0x20, 0x01, 0xDE, 0x31, 0xC9,

	// Find GetProcAddress
	0x41,
	0xAD, 0x01, 0xD8, 0x81, 0x38, 0x47, 0x65, 0x74, 0x50, 0x75, 0xF4, 0x81,
	0x78, 0x04, 0x72, 0x6F, 0x63, 0x41, 0x75, 0xEB, 0x81, 0x78, 0x08, 0x64,
	0x64, 0x72, 0x65, 0x75, 0xE2, 0x8B, 0x72, 0x24, 0x01, 0xDE, 0x66, 0x8B,
	0x0C, 0x4E, 0x49, 0x8B, 0x72, 0x1C, 0x01, 0xDE, 0x8B, 0x14, 0x8E, 0x01,
	0xDA, 0x31, 0xC9, 0x53, 0x52, 0x51,
	0x68, 0x61, 0x72, 0x79, 0x57, // aryW
	0x68, 0x4C, 0x69, 0x62, 0x72, // Libr
	0x68, 0x4C, 0x6F, 0x61, 0x64, // Load
	0x54,             // PUSH "LoadLibraryW"
	0x53, 0xFF, 0xD2, // GetProcAddress("LoadLibraryW")
	0x83, 0xC4, 0x18, // restore stack
	0xFF, 0xE0, // jmp LoadLibraryW(arg0)
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

func inject(exePath string) error {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	err := kernel32.Load()
	if err != nil {
		return err
	}

	log.Println("kernel32.dll has been loaded")

	WriteProcessMemory := kernel32.NewProc("WriteProcessMemory")
	VirtualAllocEx := kernel32.NewProc("VirtualAllocEx")
	CreateRemoteThread := kernel32.NewProc("CreateRemoteThread")
	LoadLibraryW := kernel32.NewProc("LoadLibraryW")

	for _, proc := range []*windows.LazyProc{
		WriteProcessMemory,
		VirtualAllocEx,
		CreateRemoteThread,
		LoadLibraryW,
	} {
		err = proc.Find()
		if err != nil {
			return fmt.Errorf("cannot find %q: %w", proc.Name, err)
		}
		log.Printf("%s has been found in kernel32.", proc.Name)
	}

	var si windows.StartupInfo
	var pi windows.ProcessInformation
	err = windows.CreateProcess(
		nil,
		S(exePath),
		nil, nil, false,
		windows.CREATE_SUSPENDED,
		nil,
		nil,
		&si,
		&pi,
	)
	if err != nil {
		return err
	}

	log.Printf("New process with %d has been created.", pi.ProcessId)

	defer func() {
		if pi.Process != 0 {
			windows.TerminateProcess(pi.Process, 1)
		}
	}()

	var isWow64 bool
	var xpage uintptr

	err = windows.IsWow64Process(pi.Process, &isWow64)

	var libinjector_name string
	if isWow64 {
		libinjector_name = "libinjector32.dll"
	} else {
		libinjector_name = "libinjector64.dll"
	}

	log.Printf("libinjector is %q", libinjector_name)

	page, _, err := VirtualAllocEx.Call(
		uintptr(pi.Process),
		0,
		windows.MAX_PATH,
		windows.MEM_COMMIT|windows.MEM_RESERVE,
		windows.PAGE_READWRITE,
	)
	if err != nil && err != ERROR_OKAY {
		return fmt.Errorf("cannot create an empty rw page: %w", err)
	}

	log.Printf("I have allocated a page %x inside the process.", page)

	if isWow64 {
		xpage, _, err = VirtualAllocEx.Call(
			uintptr(pi.Process),
			0,
			windows.MAX_PATH,
			windows.MEM_COMMIT|windows.MEM_RESERVE,
			windows.PAGE_EXECUTE_READWRITE,
		)
		if err != nil && err != ERROR_OKAY {
			return fmt.Errorf("cannot create an empty rwx page: %w", err)
		}

		log.Printf("I have allocated an executable page %x inside the process.", xpage)
	} else {
		xpage = LoadLibraryW.Addr()
	}

	_, _, err = WriteProcessMemory.Call(
		uintptr(pi.Process),
		page,
		uintptr(unsafe.Pointer(S(libinjector_name))),
		uintptr(len(libinjector_name)*2),
		0,
	)
	if err != nil && err != ERROR_OKAY {
		return fmt.Errorf("cannot write into process data memory: %w", err)
	}

	if isWow64 {
		_, _, err = WriteProcessMemory.Call(
			uintptr(pi.Process),
			xpage,
			uintptr(unsafe.Pointer(&shellcode[0])),
			uintptr(len(shellcode)),
			0,
		)
		if err != nil && err != ERROR_OKAY {
			return fmt.Errorf("cannot write into process code memory: %w", err)
		}

		log.Println("WriteProcessMemory has succeeded.")
	}

	var threadId uintptr

	log.Printf("LoadLibraryW addr is %x", LoadLibraryW.Addr())

	threadHandle, _, err := CreateRemoteThread.Call(
		uintptr(pi.Process),                // [in] HANDLE hProcess,
		0,                                  // [in]  LPSECURITY_ATTRIBUTES  lpThreadAttributes,
		0,                                  // [in]  SIZE_T                 dwStackSize,
		xpage,                              // [in] LPTHREAD_START_ROUTINE lpStartAddress,
		page,                               // [in] LPVOID lpParameter,
		0,                                  // [in] DWORD dwCreationFlags,
		uintptr(unsafe.Pointer(&threadId)), // [out] LPDWORD                lpThreadId
	)
	if err != nil && err != ERROR_OKAY {
		return fmt.Errorf("cannot create remote thread: %w", err)
	}

	defer windows.CloseHandle(windows.Handle(threadHandle))

	log.Printf("CreateRemoteThread has succeeded, threadid=%d, waiting for DllMain to return...", threadId)

	r, err := windows.WaitForSingleObject(
		windows.Handle(threadHandle),
		windows.INFINITE,
	)
	if err != nil {
		return fmt.Errorf("cannot wait for DllMain to finish: %w", err)
	}

	log.Printf("WaitForSingleObject returns %x", r)

	_, err = windows.ResumeThread(pi.Thread)
	if err != nil {
		return fmt.Errorf("cannot resume thread: %w", err)
	}

	pi.Process = 0

	return nil
}

// vim: ai:ts=8:sw=8:noet:syntax=go
