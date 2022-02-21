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
	"encoding/binary"
	"fmt"
	"log"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type MemoryBasicInformation struct {
	BaseAddress    uintptr
	AllocationBase windows.Handle

	AllocationProtect uint32
	PartitionID       uint16
	_dummy0           uint16

	RegionSize uintptr

	State   uint32
	Protect uint32
	Type    uint32
	_dummy1 uint32
}

type Patcher struct {
	ExeName              string
	is64                 bool
	module               windows.Handle
	hProcess             windows.Handle
	hThread              windows.Handle
	EnumProcessModulesEx *windows.LazyProc
	VirtualAllocEx       *windows.LazyProc
	VirtualFreeEx        *windows.LazyProc
	VirtualQueryEx       *windows.LazyProc
	GetModuleFileNameEx  *windows.LazyProc
	ReadProcessMemory    *windows.LazyProc
	WriteProcessMemory   *windows.LazyProc
	CreateRemoteThread   *windows.LazyProc
	SuspendThread        *windows.LazyProc
	ResumeThread         *windows.LazyProc
	GetExitCodeThread    *windows.LazyProc

	ServerPipeHandle windows.Handle
	ClientPipeHandle windows.Handle

	Scratch     uintptr
	ExecScratch uintptr
	GpaExecPage uintptr
	OpExecPage  uintptr
	WfExecPage  uintptr

	RconServer *RconServer
}

const LIST_MODULES_ALL = 0x03

type PatchState struct {
	Err      error
	Patcher  *Patcher
	result   uintptr
	String   string
	FastCall bool
}

func NewPatcher(hProcess, hThread windows.Handle, exeName string, rconPassword string) (*Patcher, error) {
	var isWow64 bool
	err := windows.IsWow64Process(hProcess, &isWow64)
	if err != nil {
		return nil, err
	}

	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	err = kernel32.Load()
	if err != nil {
		return nil, err
	}

	psapi := windows.NewLazySystemDLL("psapi.dll")
	err = psapi.Load()
	if err != nil {
		return nil, err
	}

	vax := kernel32.NewProc("VirtualAllocEx")
	err = vax.Find()
	if err != nil {
		return nil, fmt.Errorf("could not find VirtualAllocEx: %w", err)
	}

	vfx := kernel32.NewProc("VirtualFreeEx")
	err = vax.Find()
	if err != nil {
		return nil, fmt.Errorf("could not find VirtualFreeEx: %w", err)
	}

	vqx := kernel32.NewProc("VirtualQueryEx")
	err = vqx.Find()
	if err != nil {
		return nil, fmt.Errorf("could not find VirtualQueryEx: %w", err)
	}

	epmx := psapi.NewProc("EnumProcessModulesEx")
	err = epmx.Find()
	if err != nil {
		return nil, fmt.Errorf("could not find EnumProcessModulesEx: %w", err)
	}

	gmfne := psapi.NewProc("GetModuleFileNameExW")
	err = gmfne.Find()
	if err != nil {
		return nil, fmt.Errorf("could not find GetModuleFileNameEx: %w", err)
	}

	rmem := kernel32.NewProc("ReadProcessMemory")
	err = rmem.Find()
	if err != nil {
		return nil, fmt.Errorf("could not find ReadProcessMemory: %w", err)
	}

	wmem := kernel32.NewProc("WriteProcessMemory")
	err = rmem.Find()
	if err != nil {
		return nil, fmt.Errorf("could not find WriteProcessMemory: %w", err)
	}

	crt := kernel32.NewProc("CreateRemoteThread")
	err = crt.Find()
	if err != nil {
		return nil, fmt.Errorf("could not find CreateRemoteThread: %w", err)
	}

	st := kernel32.NewProc("SuspendThread")
	err = crt.Find()
	if err != nil {
		return nil, fmt.Errorf("could not find SuspendThread: %w", err)
	}

	rt := kernel32.NewProc("ResumeThread")
	err = crt.Find()
	if err != nil {
		return nil, fmt.Errorf("could not find ResumeThread: %w", err)
	}

	gect := kernel32.NewProc("GetExitCodeThread")
	err = crt.Find()
	if err != nil {
		return nil, fmt.Errorf("could not find GetExitCodeThread: %w", err)
	}

	scratch, _, err := vax.Call(
		uintptr(hProcess),
		0,
		4096,
		windows.MEM_COMMIT|windows.MEM_RESERVE,
		windows.PAGE_READWRITE,
	)
	if err != nil && err != ERROR_OKAY {
		return nil, fmt.Errorf("could not allocate the scratch page: %w", err)
	} else {
		log.Printf("scratch page is %x", scratch)
	}

	xscratch, _, err := vax.Call(
		uintptr(hProcess),
		0,
		4096,
		windows.MEM_COMMIT|windows.MEM_RESERVE,
		windows.PAGE_EXECUTE_READWRITE,
	)
	if err != nil && err != ERROR_OKAY {
		return nil, fmt.Errorf("could not allocate the executable scratch page: %w", err)
	}

	var is64 bool
	if !isWow64 {
		is64 = true
	}

	rconServer, err := NewRconServer(rconPassword)
	if err != nil {
		return nil, fmt.Errorf("cannot start rcon server: %w", err)
	}

	flags := uint32(windows.PIPE_ACCESS_INBOUND)
	flags |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE

	pipeHandle, err := windows.CreateNamedPipe(
		/* name= */ S(`\\.\pipe\zdrct-printf`),
		/* flags= */ flags,
		/* mode= */ windows.PIPE_TYPE_MESSAGE|
			windows.PIPE_READMODE_MESSAGE,
		/* maxinstances= */ 4,
		4096,
		4096,
		1000,
		nil,
	)
	if err != nil && err != ERROR_OKAY {
		return nil, fmt.Errorf("cannot create named pipe: %w", err)
	}

	go func() {
		err = windows.ConnectNamedPipe(pipeHandle, nil)
		if err != nil && err != ERROR_OKAY {
			log.Printf("cannot connect named pipe: %s", err)
			return
		}

		log.Println("Client has been connected to the pipe.")

		var done uint32
		buf := make([]byte, 4096)
		for {
			err = windows.ReadFile(
				pipeHandle,
				buf,
				&done,
				nil,
			)
			if err != nil && err != ERROR_OKAY {
				if err == windows.ERROR_BROKEN_PIPE {
					return
				}
				log.Fatalf("read error: %s", err)
			}

			log.Printf("got msg: %q", buf[:done])
		}
	}()

	patcher := &Patcher{
		is64:                 is64,
		ExeName:              exeName,
		hProcess:             hProcess,
		hThread:              hThread,
		VirtualAllocEx:       vax,
		VirtualFreeEx:        vfx,
		VirtualQueryEx:       vqx,
		EnumProcessModulesEx: epmx,
		GetModuleFileNameEx:  gmfne,
		ReadProcessMemory:    rmem,
		WriteProcessMemory:   wmem,
		CreateRemoteThread:   crt,
		SuspendThread:        st,
		ResumeThread:         rt,
		GetExitCodeThread:    gect,

		Scratch:     scratch,
		ExecScratch: xscratch,
		RconServer:  rconServer,

		ServerPipeHandle: pipeHandle,
	}

	gpa := []byte{
		/*  0 */ 0x31, 0xC9, 0x64, 0x8B, 0x41, 0x30, // Find PEB
		/*  6 */ 0x8B, 0x40, 0x0C, 0x8B, 0x70, 0x14,
		/* 12 */ 0xAD, 0x96, 0xAD, 0x8B, 0x58, 0x10, 0x8B, 0x53, 0x3C, 0x01, 0xDA, 0x8B,
		/* 24 */ 0x52, 0x78, 0x01, 0xDA, 0x8B, 0x72, 0x20, 0x01, 0xDE, 0x31, 0xC9,

		// Find GetProcAddress
		/* 35 */ 0x41,
		/* 36 */ 0xAD, 0x01, 0xD8, 0x81, 0x38, 0x47, 0x65, 0x74, 0x50, 0x75, 0xF4, 0x81,
		/* 48 */ 0x78, 0x04, 0x72, 0x6F, 0x63, 0x41, 0x75, 0xEB, 0x81, 0x78, 0x08, 0x64,
		/* 60 */ 0x64, 0x72, 0x65, 0x75, 0xE2, 0x8B, 0x72, 0x24, 0x01, 0xDE, 0x66, 0x8B,
		/* 72 */ 0x0C, 0x4E, 0x49, 0x8B, 0x72, 0x1C, 0x01, 0xDE, 0x8B, 0x14, 0x8E, 0x01,
		/* 84 */ 0xDA, 0x31, 0xC9,

		/* 87 */ 0x68, 0x00, 0x00, 0x00, 0x00, // PUSH Scratch
		/* 92 */ 0x53, // PUSH kernel32
		/* 93 */ 0xFF, 0xD2, // GetProcAddress(ebx, arg0)
		/* 95 */ 0x89, 0x05, 0, 0, 0, 0, // mov [Scratch], eax
		/*101 */ 0xC3, // ret
	}
	binary.LittleEndian.PutUint32(gpa[88:], uint32(scratch))
	binary.LittleEndian.PutUint32(gpa[97:], uint32(scratch))
	patcher.GpaExecPage, err = patcher.MakeExecPage(gpa)
	if err != nil {
		return nil, fmt.Errorf("cannot create GetProcAddress gadget: %w", err)
	}

	remoteCreateFile, err := patcher.GetRemoteProcAddr("CreateFileW")
	if err != nil {
		return nil, err
	}

	op := []byte{
		/*  0 */ 0x6A, 0, // templatefile=0
		/*  2 */ 0x6A, 0, // flags=0
		/*  4 */ 0x6A, 3, // createmode=OPEN_EXISTING
		/*  6 */ 0x6A, 0, // sa=0
		/*  8 */ 0x6A, 3, // mode=FILE_SHARE_READ|FILE_SHARE_WRITE
		/* 10 */ 0x68, 0, 0, 0, 0x40, // access=GENERIC_WRITE
		/* 15 */ 0x68, 0, 0, 0, 0, // name=Scratch
		/* 20 */ 0xBA, 0, 0, 0, 0, // CreateFileW
		/* 25 */ 0xFF, 0xD2, // CALL CreateFileW(...)
		/* 27 */ 0x89, 0x05, 0, 0, 0, 0, // mov [Scratch], eax
		/* 33 */ 0xC3, // ret
	}
	binary.LittleEndian.PutUint32(op[16:], uint32(scratch))
	binary.LittleEndian.PutUint32(op[21:], uint32(remoteCreateFile))
	binary.LittleEndian.PutUint32(op[29:], uint32(scratch))
	patcher.OpExecPage, err = patcher.MakeExecPage(op)
	if err != nil {
		return nil, fmt.Errorf("cannot create CreateFile gadget: %w", err)
	}

	patcher.ClientPipeHandle, err = patcher.CreateRemoteFileW(`\\.\pipe\zdrct-printf`)
	if err != nil {
		return nil, fmt.Errorf("cannot connect to the pipe: %w", err)
	}

	go rconServer.Start()

	return patcher, nil
}

const MEM_FREE = 0x10000

func (p *Patcher) ExecRconCommands(C_DoCommand *PatchState) {
	go func() {
		for cmd := range p.RconServer.Commands() {
			err := C_DoCommand.Call(cmd)
			if err != nil {
				log.Printf("Call has failed: %s", err)
			}
		}
	}()
}

func (p *Patcher) readptr(buf []byte, idx int) uintptr {
	if p.is64 {
		return uintptr(binary.LittleEndian.Uint64(buf[idx*8:]))
	} else {
		return uintptr(binary.LittleEndian.Uint32(buf[idx*4:]))
	}
}

func (p *Patcher) writeptr(buf []byte, idx int, ptr uintptr) {
	if p.is64 {
		binary.LittleEndian.PutUint64(buf[idx*8:], uint64(ptr))
	} else {
		binary.LittleEndian.PutUint32(buf[idx*4:], uint32(ptr))
	}
}

func (p *Patcher) MemChr(haystack uintptr, haystack_sz int, needle byte) (uintptr, error) {
	for scan := haystack; scan < haystack+uintptr(haystack_sz); scan += 4096 {
		var sz int = 4096
		if left := haystack_sz - int(scan-uintptr(haystack)); left < sz {
			sz = left
		}

		page, err := p.read(scan, sz)
		if err != nil {
			return 0, err
		}

		idx := bytes.IndexByte(page[:sz], needle)
		if idx != -1 {
			return scan + uintptr(idx), nil
		}
	}

	return 0, ErrNotFound
}

func (p *Patcher) MemMem(haystack uintptr, haystack_sz int, needle []byte) (uintptr, error) {
	for scan := haystack; scan < haystack+uintptr(haystack_sz); scan += 2048 {
		var sz int = 4096
		if left := haystack_sz - int(scan-uintptr(haystack)); left < sz {
			sz = left
		}

		if sz < len(needle)-1 {
			break
		}

		page, err := p.read(scan, sz)
		if err != nil {
			return 0, err
		}

		idx := bytes.Index(page[:sz], needle)
		if idx != -1 {
			return scan + uintptr(idx), nil
		}
	}

	return 0, ErrNotFound
}

func (p *Patcher) search_string_cb(ptr uintptr, size int, arg interface{}) (interface{}, error) {
	addr, err := p.MemMem(ptr, size, arg.([]byte))
	if err != nil {
		return nil, err
	}
	return addr, nil
}

type ArgValue struct {
	Func  uintptr
	Arg   int
	Value uintptr
}

func (p *Patcher) search_load_arg(ptr uintptr, size int, arg interface{}) (interface{}, error) {
	pattern := []byte{
		0xC7, 0x44, 0x24, 0x00, 0x00, 0x00, 0x00, 0x00,
	} // mov ss:[esp+disp8], imm32
	av := arg.(*ArgValue)
	if av.Func < ptr || av.Func > ptr+uintptr(size) {
		return nil, ErrNotFound
	}
	if p.is64 {
		pattern[3] = byte(av.Arg * 8)
	} else {
		pattern[3] = byte(av.Arg * 4)
	}
	binary.LittleEndian.PutUint32(pattern[4:], uint32(av.Value))
	m, err := p.MemMem(av.Func, size-int(av.Func-ptr)-64, pattern)
	buf := [64]byte{}
	_, _, err = p.ReadProcessMemory.Call(
		uintptr(p.hProcess),
		m,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0,
	)
	if err != nil && err != ERROR_OKAY {
		return nil, fmt.Errorf(
			"ReadProcessMemory(%x, %x, nSize=%d) has failed: %w",
			p.hProcess,
			m,
			len(buf),
			err,
		)
	}
	idx := bytes.IndexByte(buf[:], 0xE8) /* call */
	if idx == -1 {
		return nil, ErrNotFound
	}
	offset := binary.LittleEndian.Uint32(buf[idx+1:])
	if offset > 0x80000000 {
		m -= uintptr(0x100000000 - uint64(offset))
	} else {
		m += uintptr(offset)
	}
	target := m + uintptr(idx+5)
	if (target & 0xF) != 0 {
		return nil, ErrNotFound
	}

	return target, nil
}

func (p *Patcher) search_mul_add(ptr uintptr, size int, arg interface{}) (interface{}, error) {
	pattern := []byte{
		/* 00 */ 0x89, 0x44, 0x24, 0x04, /* mov ss:[esp+4], eax */
		/* 04 */ 0x69, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, /* imul eax, ds:[imm32], imm32 */
		/* 0e */ 0x05, 0x00, 0x00, 0x00, 0x00, /* add eax, imm32 */
		/* 13 */ 0x89, 0x04, 0x24, /* mov ss:[esp], eax */
		/* 16 */ 0xE8, 0x00, 0x00, 0x00, 0x00, /* call rel32 */
	}
	if arg.(uintptr) < ptr || arg.(uintptr) >= ptr+uintptr(size) {
		return nil, ErrNotFound
	}

	m, err := p.MemMem(arg.(uintptr), 64, pattern[:6])
	if err != nil {
		return nil, err
	}

	buf := make([]byte, len(pattern))
	_, _, err = p.ReadProcessMemory.Call(
		uintptr(p.hProcess),
		m,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0,
	)
	if err != nil && err != ERROR_OKAY {
		return nil, fmt.Errorf(
			"ReadProcessMemory(%x, %x, nSize=%d) has failed: %w",
			p.hProcess,
			m,
			len(buf),
			err,
		)
	}

	if buf[0xe] != pattern[0xe] {
		return nil, ErrNotFound
	}

	if bytes.Compare(buf[0x13:0x17], pattern[0x13:0x17]) != 0 {
		return nil, ErrNotFound
	}

	// TODO: check if it is 64-bit safe
	return uintptr(binary.LittleEndian.Uint32(buf[0xe+1:])), nil
}

func (p *Patcher) MakeExecPage(code []byte) (uintptr, error) {
	page_size := len(code) + 4095
	page_size -= page_size % 4096
	addr, _, err := p.VirtualAllocEx.Call(
		uintptr(p.hProcess),
		0,
		uintptr(page_size),
		windows.MEM_COMMIT|windows.MEM_RESERVE,
		windows.PAGE_EXECUTE_READWRITE,
	)
	if err != nil && err != ERROR_OKAY {
		return 0, fmt.Errorf("could not allocate the executable scratch page: %w", err)
	}

	_, _, err = p.WriteProcessMemory.Call(
		uintptr(p.hProcess),
		addr,
		uintptr(unsafe.Pointer(&code[0])),
		uintptr(len(code)),
		0,
	)

	if err != nil && err != ERROR_OKAY {
		p.VirtualFreeEx.Call(
			uintptr(p.hProcess),
			addr,
			uintptr(page_size),
			0,
		)

		return 0, fmt.Errorf("cannot write to new page: %w", err)
	}

	return addr, nil
}

func (p *Patcher) search_data_ref(ptr uintptr, size int, arg interface{}) (interface{}, error) {
	var prologue, prefix, pattern []byte
	var err error

	if p.is64 {
		// prologue = []byte{0x55, 0x48, 0x89, 0xe5} // push rbp; mov rbp, rsp
		if runtime.GOOS == "windows" {
			prefix = []byte{0xcc, 0xcc, 0xcc}  // nop; nop; nop
			pattern = []byte{0x48, 0x8d, 0x0d} // lea rcx, [rip+off32]
		} else {
			prefix = []byte{0}
			pattern = []byte{0x48, 0x8d, 0x3d} // lea rdi, [rip+off32]
		}
		for scan := ptr; scan < ptr+uintptr(size); ptr += 1 {
			scan, err = p.MemMem(scan, size-int(scan-ptr), pattern)
			if err != nil {
				return nil, err
			}
			scan += uintptr(len(pattern))
			target, err := p.readS32(scan, scan+4)
			if err != nil {
				return nil, err
			}
			if target != arg.(uintptr) {
				continue
			}

			call, err := p.MemChr(scan, 64, 0xe8) // call rel32
			if err != nil {
				if err == ErrNotFound {
					continue
				}
				return nil, err
			}

			call += 1
			call, err = p.readS32(call, call+4)
			if err != nil {
				return nil, err
			}

			buf, err := p.read(call-uintptr(len(prefix)), len(prefix))
			if err != nil {
				return nil, err
			}

			if bytes.Compare(buf, prefix) == 0 {
				return call, nil
			}

			buf, err = p.read(call, len(prologue))
			if bytes.Compare(buf, prologue) == 0 {
				return call, nil
			}
		}
	} else {
		pattern = []byte{0x68, 0x00, 0x00, 0x00, 0x00, 0xe8} // push imm32; call rel32
		binary.LittleEndian.PutUint32(pattern[1:], uint32(arg.(uintptr)))
		call, err := p.MemMem(ptr, size, pattern)
		if err != nil {
			return nil, err
		}
		call += uintptr(len(pattern))
		target, err := p.readS32(call, call+4)
		if err != nil {
			return nil, err
		}

		if (target & 0xF) != 0 {
			return nil, ErrNotFound
		}

		return target, nil
	}

	return nil, ErrNotFound
}

func (p *Patcher) search_data_ref_fast(ptr uintptr, size int, arg interface{}) (interface{}, error) {
	pattern := []byte{0xb9, 0x00, 0x00, 0x00, 0x00, 0xe8} // mov ecx, imm32; call rel32
	binary.LittleEndian.PutUint32(pattern[1:], uint32(arg.(uintptr)))
	call, err := p.MemMem(ptr, size, pattern)
	if err != nil {
		return nil, err
	}
	call += uintptr(len(pattern))
	target, err := p.readS32(call, call+4)
	if err != nil {
		return nil, err
	}
	if (target & 0xF) != 0 {
		return nil, ErrNotFound
	}
	return target, nil
}

func (p *Patcher) search_data_load(ptr uintptr, size int, arg interface{}) (interface{}, error) {
	buf := []byte{0xA1} /* mov eax, ds:[mem32] */
	buf = append(buf, p.makeptrs(1)...)
	p.writeptr(buf[1:], 0, arg.(uintptr))
	return p.MemMem(ptr, size, buf)
}

func (p *Patcher) makeptrs(nptrs int) []byte {
	if p.is64 {
		return make([]byte, nptrs*8)
	} else {
		return make([]byte, nptrs*4)
	}
}

func (p *Patcher) read(ptr uintptr, size int) ([]byte, error) {
	buf := make([]byte, size)
	_, _, err := p.ReadProcessMemory.Call(
		uintptr(p.hProcess),
		ptr,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0,
	)
	if err != nil && err != ERROR_OKAY {
		return nil, fmt.Errorf(
			"ReadProcessMemory(%x, %x, nSize=%d) has failed: %w",
			p.hProcess,
			ptr,
			len(buf),
			err,
		)
	}

	return buf, nil
}

func (p *Patcher) readU32(ptr uintptr) (uint32, error) {
	buf, err := p.read(ptr, 4)
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint32(buf[:]), nil
}

func (p *Patcher) readS32(ptr, offset uintptr) (uintptr, error) {
	buf := [4]byte{}
	_, _, err := p.ReadProcessMemory.Call(
		uintptr(p.hProcess),
		ptr,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0,
	)
	if err != nil && err != ERROR_OKAY {
		return 0, fmt.Errorf(
			"ReadProcessMemory(%x, %x, nSize=%d) has failed: %w",
			p.hProcess,
			ptr,
			len(buf),
			err,
		)
	}

	ui32 := binary.LittleEndian.Uint32(buf[:])
	if ui32 >= 0x80000000 {
		return offset - uintptr(0x100000000-uint64(ui32)), nil
	} else {
		return offset + uintptr(ui32), nil
	}
}

func (p *Patcher) search_data_store(ptr uintptr, size int, arg interface{}) (interface{}, error) {
	var err error
	pattern := []byte{0xC7, 0x05} /* mov ds:[mem32], imm32 */
	buf := p.makeptrs(2)
	for scan := ptr; scan < ptr+uintptr(size); scan += 1 {
		scan, err = p.MemMem(
			scan,
			size-int(scan-ptr)-len(buf),
			pattern,
		)
		if err != nil {
			return nil, err
		}
		_, _, err := p.ReadProcessMemory.Call(
			uintptr(p.hProcess),
			scan+uintptr(len(pattern)),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
			0,
		)
		if err != nil && err != ERROR_OKAY {
			return nil, fmt.Errorf(
				"ReadProcessMemory(%x, %x, nSize=%d) has failed: %w",
				p.hProcess,
				scan+uintptr(len(pattern)),
				len(buf),
				err,
			)
		}
		if p.readptr(buf, 1) == arg.(uintptr) {
			return p.readptr(buf, 0), nil
		}
	}

	return nil, ErrNotFound
}

func (p *Patcher) SearchFunc(ptr uintptr) (uintptr, error) {
	ptr -= ptr & 0xF
	buf := [8]byte{}
	for i := 0; i < 32; i++ {
		_, _, err := p.ReadProcessMemory.Call(
			uintptr(p.hProcess),
			ptr-4,
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
			0,
		)
		if err != nil && err != ERROR_OKAY {
			return 0, fmt.Errorf(
				"ReadProcessMemory(%x, %x, nSize=%d) has failed: %w",
				p.hProcess,
				ptr-4,
				len(buf),
				err,
			)
		}
		if buf[4] != 0x55 && buf[3] != 0x90 && buf[3] != 0xC3 {
			ptr -= 0x10
			continue
		}

		return ptr, nil
	}

	return 0, ErrNotFound
}

func (p *Patcher) ScanString(s string) *PatchState {
	result, err := p.scan(windows.PAGE_READONLY, p.search_string_cb, []byte(s))
	if err != nil {
		return &PatchState{
			Patcher: p,
			Err:     fmt.Errorf("error while searching for %q: %w", s, err),
		}
	}

	return &PatchState{
		Patcher: p,
		String:  fmt.Sprintf("%q", s),
		result:  result.(uintptr),
	}
}

func (ps *PatchState) PatchPrintf() error {
	if ps.Err != nil {
		return ps.Err
	}

	buf := make([]byte, 54)
	for i := range buf {
		buf[i] = 0x90 // NOP
	}

	for printf_call := ps.result; ; printf_call += 1 {
		printf_buf, err := ps.Patcher.read(printf_call-1, 6)
		if err != nil {
			return err
		}

		if printf_buf[1] == 0xCC {
			break
		}

		if printf_buf[1] != 0xB9 { // mov ecx, imm32
			continue
		}

		if (printf_buf[0] & 0xF0) != 0x50 { // PUSH reg
			continue
		}

		var imm32 uint32
		buf[0] = 0x60 // PUSHA
		buf[1] = 0x68 // PUSH 0
		binary.LittleEndian.PutUint32(buf[2:], imm32)
		buf[6] = 0x68 // PUSH 0
		binary.LittleEndian.PutUint32(buf[7:], imm32)
		buf[11] = 0x54 // PUSH esp
		buf[16] = 0x68 // TODO: PUSH printf_callback
		imm32 = 0
		buf[21] = 0x68 // PUSH STACK_SIZE
		binary.LittleEndian.PutUint32(buf[22:], imm32)
		buf[26] = 0x68 // PUSH 0
		binary.LittleEndian.PutUint32(buf[27:], imm32)

		imm32 = 0      // TODO: ((long) &CreateThread) - ((long) &buf[36]);
		buf[31] = 0xE8 // CALL CreateThread
		binary.LittleEndian.PutUint32(buf[32:], imm32)

		imm32 = windows.INFINITE
		buf[36] = 0x68 // PUSH INFINITE
		binary.LittleEndian.PutUint32(buf[37:], imm32)
		buf[41] = 0x50 // PUSH handle
		imm32 = 0      // TODO: ((long) &WaitForSingleObject) - ((long) &buf[47]);
		buf[42] = 0xE8 // CALL WaitForSingleObject
		binary.LittleEndian.PutUint32(buf[43:], imm32)
		buf[47] = 0x61 // POPA
		binary.LittleEndian.PutUint32(buf[48:], uint32(printf_call))
		copy(buf[48:53], printf_buf[1:6]) // orig MOV ecx, imm32
		buf[53] = 0xC3                    // RET

		trampoline, err := ps.Patcher.MakeExecPage(buf)
		if err != nil {
			return err
		}

		call_offset := trampoline - (printf_call + 5)
		call_buf := [5]byte{0xE8}
		binary.LittleEndian.PutUint32(call_buf[1:], uint32(call_offset))

		_, _, err = ps.Patcher.WriteProcessMemory.Call(
			uintptr(ps.Patcher.hProcess),
			ps.result,
			uintptr(unsafe.Pointer(&call_buf[0])),
			uintptr(len(call_buf)),
			0,
		)
		if err != nil && err != ERROR_OKAY {
			return err
		}

		return nil
	}

	return fmt.Errorf("cannot find MOV ecx, imm32 in Printf: %x", ps.result)
}

func (ps *PatchState) StoreRef() *PatchState {
	if ps.Err != nil {
		return ps
	}

	result, err := ps.Patcher.scan(windows.PAGE_EXECUTE_READ, ps.Patcher.search_data_store, ps.result)
	if err != nil {
		return &PatchState{
			Patcher: ps.Patcher,
			Err: fmt.Errorf(
				"error while searching data store references to %s: %w",
				ps.String,
				err,
			),
		}
	}
	return &PatchState{
		Patcher: ps.Patcher,
		String:  "store ref to " + ps.String,
		result:  result.(uintptr),
	}
}

func (ps *PatchState) LoadDataRef() *PatchState {
	if ps.Err != nil {
		return ps
	}

	var fastcall bool
	result, err := ps.Patcher.scan(windows.PAGE_EXECUTE_READ, ps.Patcher.search_data_ref, ps.result)
	if ps.Patcher.is64 == false && err == ErrNotFound {
		result, err = ps.Patcher.scan(windows.PAGE_EXECUTE_READ, ps.Patcher.search_data_ref_fast, ps.result)
		if nil == err {
			fastcall = true
		}
	}
	if err != nil {
		return &PatchState{
			Patcher: ps.Patcher,
			Err: fmt.Errorf(
				"error while searching data references to %s: %w",
				ps.String,
				err,
			),
		}
	}
	return &PatchState{
		Patcher:  ps.Patcher,
		String:   "store ref to " + ps.String,
		result:   result.(uintptr),
		FastCall: fastcall,
	}
}

func (p *Patcher) CreateRemoteFileW(filename string) (windows.Handle, error) {
	u, err := syscall.UTF16FromString(filename)
	if err != nil {
		return 0, fmt.Errorf("cannot convert to UTF-16: %q: %w", filename, err)
	}

	_, _, err = p.WriteProcessMemory.Call(
		uintptr(p.hProcess),
		p.Scratch,
		uintptr(unsafe.Pointer(&u[0])),
		uintptr(len(u)*2),
		0,
	)
	if err != nil && err != ERROR_OKAY {
		return 0, fmt.Errorf("cannot write to scratch page: %w", err)
	}

	var threadId uintptr
	threadHandle, _, err := p.CreateRemoteThread.Call(
		uintptr(p.hProcess),                // [in] HANDLE hProcess,
		0,                                  // [in]  LPSECURITY_ATTRIBUTES  lpThreadAttributes,
		0,                                  // [in]  SIZE_T                 dwStackSize,
		p.OpExecPage,                       // [in] LPTHREAD_START_ROUTINE lpStartAddress,
		p.Scratch,                          // [in] LPVOID lpParameter,
		0,                                  // [in] DWORD dwCreationFlags,
		uintptr(unsafe.Pointer(&threadId)), // [out] LPDWORD                lpThreadId
	)
	if err != nil && err != ERROR_OKAY {
		return 0, fmt.Errorf("cannot create remote thread: %w", err)
	}

	defer windows.CloseHandle(windows.Handle(threadHandle))

	_, err = windows.WaitForSingleObject(
		windows.Handle(threadHandle),
		windows.INFINITE,
	)
	if err != nil && err != ERROR_OKAY {
		return 0, fmt.Errorf("cannot wait for thread to finish: %w", err)
	}

	remoteHandle, err := p.readU32(p.Scratch)
	if err != nil && err != ERROR_OKAY {
		return 0, fmt.Errorf("cannot read scratch word: %w", err)
	}

	if remoteHandle == 0xFFFFFFFF {
		return 0, fmt.Errorf("cannot open file %q", filename)
	}

	return windows.Handle(remoteHandle), nil
}

func (p *Patcher) GetRemoteProcAddr(symbol string) (uintptr, error) {
	buf := append([]byte(symbol), 0)

	_, _, err := p.WriteProcessMemory.Call(
		uintptr(p.hProcess),
		p.Scratch,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0,
	)
	if err != nil && err != ERROR_OKAY {
		return 0, fmt.Errorf("cannot write to scratch page: %w", err)
	}

	var threadId uintptr
	threadHandle, _, err := p.CreateRemoteThread.Call(
		uintptr(p.hProcess),                // [in] HANDLE hProcess,
		0,                                  // [in]  LPSECURITY_ATTRIBUTES  lpThreadAttributes,
		0,                                  // [in]  SIZE_T                 dwStackSize,
		p.GpaExecPage,                      // [in] LPTHREAD_START_ROUTINE lpStartAddress,
		p.Scratch,                          // [in] LPVOID lpParameter,
		0,                                  // [in] DWORD dwCreationFlags,
		uintptr(unsafe.Pointer(&threadId)), // [out] LPDWORD                lpThreadId
	)
	if err != nil && err != ERROR_OKAY {
		return 0, fmt.Errorf("cannot create remote thread: %w", err)
	}

	defer windows.CloseHandle(windows.Handle(threadHandle))

	_, err = windows.WaitForSingleObject(
		windows.Handle(threadHandle),
		windows.INFINITE,
	)
	if err != nil && err != ERROR_OKAY {
		return 0, fmt.Errorf("cannot wait for thread to finish: %w", err)
	}

	remote_addr, err := p.readU32(p.Scratch)
	if err != nil && err != ERROR_OKAY {
		return 0, fmt.Errorf("cannot read scratch word: %w", err)
	}

	if remote_addr == 0 {
		return 0, fmt.Errorf("cannot find symbol %q in kernel32", symbol)
	}

	return uintptr(remote_addr), nil
}

func (ps *PatchState) Call(s string) (err error) {
	if ps.Err != nil {
		return ps.Err
	}

	buf := append([]byte(s), 0)

	_, _, err = ps.Patcher.WriteProcessMemory.Call(
		uintptr(ps.Patcher.hProcess),
		ps.Patcher.Scratch,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0,
	)
	if err != nil && err != ERROR_OKAY {
		err = fmt.Errorf("cannot write to scratch page: %w", err)
		return
	}

	if ps.FastCall {
		buf := []byte{
			0xb9, 0x00, 0x00, 0x00, 0x00, /* mov ecx, imm32 */
			0xe9, 0x00, 0x00, 0x00, 0x00, /* jmp imm32 */
		}
		binary.LittleEndian.PutUint32(buf[1:], uint32(ps.Patcher.Scratch))
		binary.LittleEndian.PutUint32(buf[6:], uint32(ps.result)-uint32(ps.Patcher.ExecScratch+uintptr(len(buf))))

		_, _, err = ps.Patcher.WriteProcessMemory.Call(
			uintptr(ps.Patcher.hProcess),
			ps.Patcher.ExecScratch,
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
			0,
		)
		if err != nil && err != ERROR_OKAY {
			err = fmt.Errorf("cannot write to exec scratch page: %w", err)
			return
		}
	}

	_, _, err = ps.Patcher.SuspendThread.Call(uintptr(ps.Patcher.hThread))
	if err != nil && err != ERROR_OKAY {
		err = fmt.Errorf("cannot suspend thread: %w", err)
		return
	}

	defer func() {
		_, _, rerr := ps.Patcher.ResumeThread.Call(uintptr(ps.Patcher.hThread))
		if rerr != nil && rerr != ERROR_OKAY {
			if err != nil && err != ERROR_OKAY {
				err = fmt.Errorf("error while recovering from %s, cannot resume thread: %w", err.Error(), rerr)
			} else {
				err = fmt.Errorf("cannot resume thread: %w", err)
			}
		}
	}()

	var threadId uintptr
	var threadHandle uintptr
	addr := ps.result

	if ps.FastCall {
		addr = ps.Patcher.ExecScratch
	}

	threadHandle, _, err = ps.Patcher.CreateRemoteThread.Call(
		uintptr(ps.Patcher.hProcess),       // [in] HANDLE hProcess,
		0,                                  // [in]  LPSECURITY_ATTRIBUTES  lpThreadAttributes,
		0,                                  // [in]  SIZE_T                 dwStackSize,
		addr,                               // [in] LPTHREAD_START_ROUTINE lpStartAddress,
		ps.Patcher.Scratch,                 // [in] LPVOID lpParameter,
		0,                                  // [in] DWORD dwCreationFlags,
		uintptr(unsafe.Pointer(&threadId)), // [out] LPDWORD                lpThreadId
	)
	if err != nil && err != ERROR_OKAY {
		err = fmt.Errorf("cannot create remote thread: %w", err)
		return
	}

	defer windows.CloseHandle(windows.Handle(threadHandle))

	_, err = windows.WaitForSingleObject(
		windows.Handle(threadHandle),
		windows.INFINITE,
	)
	if err != nil && err != ERROR_OKAY {
		err = fmt.Errorf("cannot wait for thread to finish: %w", err)
		return
	}

	return
}

func (ps *PatchState) LoadRef() *PatchState {
	if ps.Err != nil {
		return ps
	}

	result, err := ps.Patcher.scan(windows.PAGE_EXECUTE_READ, ps.Patcher.search_data_load, ps.result)
	if err != nil {
		return &PatchState{
			Patcher: ps.Patcher,
			Err: fmt.Errorf(
				"error while searching data load references to %s: %w",
				ps.String,
				err,
			),
		}
	}
	return &PatchState{
		Patcher: ps.Patcher,
		String:  "load ref to " + ps.String,
		result:  result.(uintptr),
	}
}

func (ps *PatchState) MulAdd() *PatchState {
	if ps.Err != nil {
		return ps
	}

	result, err := ps.Patcher.scan(windows.PAGE_EXECUTE_READ, ps.Patcher.search_mul_add, ps.result)
	if err != nil {
		return &PatchState{
			Patcher: ps.Patcher,
			Err: fmt.Errorf(
				"error while searching mul+add references to %s: %w",
				ps.String,
				err,
			),
		}
	}
	return &PatchState{
		Patcher: ps.Patcher,
		String:  "mul+add ref to " + ps.String,
		result:  result.(uintptr),
	}
}

func (ps *PatchState) FuncAlign() *PatchState {
	if ps.Err != nil {
		return ps
	}

	result, err := ps.Patcher.SearchFunc(ps.result)
	if err != nil {
		return &PatchState{
			Patcher: ps.Patcher,
			Err: fmt.Errorf(
				"error while searching the beginning of the function near %s: %w",
				ps.String,
				err,
			),
		}
	}

	return &PatchState{
		Patcher: ps.Patcher,
		String:  "aligned function near " + ps.String,
		result:  result,
	}
}

func (p *Patcher) Nil() *PatchState {
	return &PatchState{Patcher: p}
}

func (ps *PatchState) ArgRef(arg int, ref *PatchState) *PatchState {
	if ps.Err != nil {
		return ps
	}

	if ref.Err != nil {
		return ps
	}

	result, err := ps.Patcher.scan(windows.PAGE_EXECUTE_READ, ps.Patcher.search_load_arg, &ArgValue{
		Func:  ps.result,
		Arg:   arg,
		Value: ref.result,
	})
	if err != nil {
		return &PatchState{
			Patcher: ps.Patcher,
			Err: fmt.Errorf(
				"error while searching for the reference of arg %d with value %s in %s: %w",
				arg,
				ref.String,
				ps.String,
				err,
			),
		}
	}

	return &PatchState{
		Patcher: ps.Patcher,
		result:  result.(uintptr),
		String: fmt.Sprintf(
			"arg %d ref with value %s in %s",
			arg,
			ref.String,
			ps.String,
		),
	}
}

func (ps *PatchState) Result() (uintptr, error) {
	if ps.Err != nil {
		return 0, ps.Err
	}

	return ps.result, nil
}

func (ps *PatchState) Error() error {
	return ps.Err
}

func (p *Patcher) scan(perm_filter uint32, cb func(ptr uintptr, size int, arg interface{}) (interface{}, error), arg interface{}) (interface{}, error) {
	var pnext uintptr
	var mbi MemoryBasicInformation

	for {
		sz, _, err := p.VirtualQueryEx.Call(uintptr(p.hProcess), pnext, uintptr(unsafe.Pointer(&mbi)), unsafe.Sizeof(mbi))
		if sz == 0 {
			break
		}
		if err != nil && err != ERROR_OKAY {
			return nil, fmt.Errorf("VirtualQueryEx has failed: %w", err)
		}
		if sz != unsafe.Sizeof(mbi) {
			return nil, fmt.Errorf("Unexpected result from VirtualQueryEx: %d != %d", sz, unsafe.Sizeof(mbi))
		}

		pnext = mbi.BaseAddress + mbi.RegionSize

		if mbi.State == MEM_FREE {
			continue
		}

		if mbi.Protect != perm_filter {
			continue
		}

		result, err := cb(mbi.BaseAddress, int(mbi.RegionSize), arg)
		if err == ErrNotFound {
			continue
		} else if err != nil {
			return nil, err
		}

		return result, nil
	}

	return nil, ErrNotFound
}

func (p *Patcher) Shutdown() {
	windows.CloseHandle(p.ServerPipeHandle)
}

var ErrNotFound = fmt.Errorf("not found")

// vim: ai:ts=8:sw=8:noet:syntax=go
