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
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"syscall"
)

type Patcher struct {
	ExeName  string
	is64     bool
	pid      int
	attached bool

	Scratch     uintptr
	Scratch2    uintptr
	ExecScratch uintptr
	Stack       uintptr

	RconServer *RconServer
}

type PatchState struct {
	Err     error
	Patcher *Patcher
	result  uintptr
	String  string
}

func (p *Patcher) Exec(rax uintptr) (uintptr, error) {
	var regs syscall.PtraceRegs
	if !p.attached {
		err := syscall.PtraceAttach(p.pid)
		if err != nil {
			return 0, fmt.Errorf("cannot attach: %w", err)
		}
		_, err = syscall.Wait4(p.pid, nil, 0, nil)
		if err != nil {
			return 0, fmt.Errorf("cannot wait: %w", err)
		}

		defer func() {
			err := syscall.PtraceDetach(p.pid)
			if err != nil {
				log.Printf("cannot detach: %s", err)
			}
		}()
	}
	err := syscall.PtraceGetRegs(p.pid, &regs)
	if err != nil {
		return 0, fmt.Errorf("cannot read regs: %w", err)
	}
	defer func() {
		err = syscall.PtraceSetRegs(p.pid, &regs)
		if err != nil {
			log.Printf("cannot write regs back: %s", err)
		}
	}()
	new_regs := regs
	new_regs.SetPC(uint64(p.ExecScratch))
	new_regs.Rax = uint64(rax)
	err = syscall.PtraceSetRegs(p.pid, &new_regs)
	if err != nil {
		return 0, fmt.Errorf("cannot set new regs: %w", err)
	}
	defer func() {
		rerr := syscall.PtraceSetRegs(p.pid, &regs)
		if rerr != nil {
			log.Printf("cannot restore regs: %s", err)
		}
	}()

	err = syscall.PtraceCont(p.pid, 0)
	if err != nil {
		return 0, err
	}

	var wstatus syscall.WaitStatus
	for {
		_, err = syscall.Wait4(p.pid, &wstatus, 0, nil)
		if err != nil {
			return 0, fmt.Errorf("cannot wait: %w", err)
		}

		if wstatus.Stopped() && wstatus.StopSignal() == syscall.SIGTRAP {
			break
		}

		if wstatus.Exited() {
			break
		}
	}

	err = syscall.PtraceGetRegs(p.pid, &new_regs)
	if err != nil {
		return 0, fmt.Errorf("cannot read regs after call: %w", err)
	}

	return uintptr(new_regs.Rax), nil
}
func (p *Patcher) Syscall(nr uint64, args ...uintptr) (uintptr, error) {
	var regs syscall.PtraceRegs
	if !p.attached {
		err := syscall.PtraceAttach(p.pid)
		if err != nil {
			return 0, err
		}

		defer syscall.PtraceDetach(p.pid)
	}
	err := syscall.PtraceGetRegs(p.pid, &regs)
	if err != nil {
		return 0, fmt.Errorf("cannot read regs: %w", err)
	}
	orig := [8]byte{}
	_, err = syscall.PtracePeekData(p.pid, uintptr(regs.PC()), orig[:])
	if err != nil {
		return 0, fmt.Errorf("cannot peek %x: %w", regs.PC(), err)
	}
	mod := orig
	mod[0] = 0x0f
	mod[1] = 0x05
	mod[2] = 0xcc
	_, err = syscall.PtracePokeData(p.pid, uintptr(regs.PC()), mod[:])
	if err != nil {
		return 0, fmt.Errorf("cannot poke %x: %w", regs.PC(), err)
	}
	defer func() {
		_, err := syscall.PtracePokeData(p.pid, uintptr(regs.PC()), orig[:])
		if err != nil {
			log.Printf("cannot poke back to %x: %s", regs.PC(), err)
		}

		err = syscall.PtraceSetRegs(p.pid, &regs)
		if err != nil {
			log.Printf("cannot write regs back: %s", err)
		}

		if !p.attached {
			err = syscall.PtraceCont(p.pid, 0)
			if err != nil {
				log.Printf("cannot continue: %s", err)
			}
		}
	}()
	scregs := regs
	scregs.Rax = nr
	for i, v := range args {
		switch i {
		case 0:
			scregs.Rdi = uint64(v)
		case 1:
			scregs.Rsi = uint64(v)
		case 2:
			scregs.Rdx = uint64(v)
		case 3:
			scregs.R10 = uint64(v)
		case 4:
			scregs.R8 = uint64(v)
		case 5:
			scregs.R9 = uint64(v)
		default:
			return 0, fmt.Errorf("too many arguments: %v", args)
		}
	}
	err = syscall.PtraceSetRegs(p.pid, &scregs)
	if err != nil {
		return 0, fmt.Errorf("cannot set new regs: %w", err)
	}

	err = syscall.PtraceCont(p.pid, 0)
	if err != nil {
		return 0, err
	}

	_, err = syscall.Wait4(p.pid, nil, 0, nil)
	if err != nil {
		return 0, fmt.Errorf("cannot wait: %w", err)
	}

	err = syscall.PtraceGetRegs(p.pid, &scregs)
	if err != nil {
		return 0, fmt.Errorf("cannot read regs after syscall: %w", err)
	}

	if int64(scregs.Rax) >= -255 && int64(scregs.Rax) < 0 {
		return 0, syscall.Errno(uintptr(-int64(scregs.Rax)))
	}

	return uintptr(scregs.Rax), nil
}

const STACK_SIZE = 8 << 20

func NewPatcher(pid int, exeName string, rconPassword string) (*Patcher, error) {
	var err error
	patcher := &Patcher{
		ExeName:  exeName,
		pid:      pid,
		attached: true,
		is64:     true, // FIXME: detect 32-bit executables
	}

	err = syscall.PtraceAttach(pid)
	if err != nil {
		return nil, fmt.Errorf("cannot attach ptrace: %w", err)
	}
	_, err = syscall.Wait4(pid, nil, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("cannot wait: %w", err)
	}

	patcher.Scratch, err = patcher.Syscall(
		syscall.SYS_MMAP,
		0,    // addr
		4096, // length
		uintptr(syscall.PROT_READ|syscall.PROT_WRITE), // prot
		uintptr(syscall.MAP_ANON|syscall.MAP_PRIVATE), // flags
		uintptr(0xffffffffffffffff),                   // fd: -1
		0,                                             // offset
	)
	if err != nil {
		return nil, fmt.Errorf("cannot allocate rw page: %w", err)
	}

	patcher.Scratch2, err = patcher.Syscall(
		syscall.SYS_MMAP,
		0,    // addr
		4096, // length
		uintptr(syscall.PROT_READ|syscall.PROT_WRITE), // prot
		uintptr(syscall.MAP_ANON|syscall.MAP_PRIVATE), // flags
		uintptr(0xffffffffffffffff),                   // fd: -1
		0,                                             // offset
	)
	if err != nil {
		return nil, fmt.Errorf("cannot allocate another rw page: %w", err)
	}

	patcher.ExecScratch, err = patcher.Syscall(
		syscall.SYS_MMAP,
		0,    // addr
		4096, // length
		uintptr(syscall.PROT_READ|syscall.PROT_EXEC),  // prot
		uintptr(syscall.MAP_ANON|syscall.MAP_PRIVATE), // flags
		uintptr(0xffffffffffffffff),                   // fd: -1
		0,                                             // offset
	)
	if err != nil {
		return nil, fmt.Errorf("cannot allocate exec page: %w", err)
	}

	patcher.Stack, err = patcher.Syscall(
		syscall.SYS_MMAP,
		0,          // addr
		STACK_SIZE, // length
		uintptr(syscall.PROT_READ|syscall.PROT_WRITE),                   // prot
		uintptr(syscall.MAP_ANON|syscall.MAP_PRIVATE|syscall.MAP_STACK), // flags
		uintptr(0xffffffffffffffff),                                     // fd: -1
		0,                                                               // offset
	)
	if err != nil {
		return nil, fmt.Errorf("cannot allocate stack: %w", err)
	}

	log.Printf("ExecScratch %x", patcher.ExecScratch)
	log.Printf("Scratch %x", patcher.Scratch)
	log.Printf("Stack %x", patcher.Stack)

	pid_, err := patcher.Syscall(syscall.SYS_GETPID)
	if err != nil {
		return nil, fmt.Errorf("getpid has failed: %w", err)
	}
	if pid != int(pid_) {
		return nil, fmt.Errorf("getpid assertion has failed: %d != %d", pid, pid_)
	}

	err = syscall.PtraceDetach(pid)
	if err != nil {
		return nil, fmt.Errorf("cannot detach: %w", err)
	}

	patcher.attached = false

	patcher.RconServer, err = NewRconServer(rconPassword)
	if err != nil {
		return nil, fmt.Errorf("cannot start rcon server: %w", err)
	}

	go patcher.RconServer.Start()

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
	buf, err := p.read(m, 64)
	if err != nil {
		return nil, err
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
		/*  0 */ 0x89, 0x44, 0x24, 0x04, /* mov ss:[esp+4], eax */
		/*  4 */ 0x69, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, /* imul eax, ds:[imm32], imm32 */
		/* 14 */ 0x05, 0x00, 0x00, 0x00, 0x00, /* add eax, imm32 */
		/* 19 */ 0x89, 0x04, 0x24, /* mov ss:[esp], eax */
		/* 22 */ 0xE8, 0x00, 0x00, 0x00, 0x00, /* call rel32 */
	}
	if arg.(uintptr) < ptr || arg.(uintptr) >= ptr+uintptr(size) {
		return nil, ErrNotFound
	}

	m, err := p.MemMem(arg.(uintptr), 64, pattern[:6])
	if err != nil {
		return nil, err
	}

	buf, err := p.read(m, len(pattern))
	if err != nil {
		return nil, err
	}

	if buf[0xe] != pattern[0xe] {
		return nil, ErrNotFound
	}

	if bytes.Compare(buf[0x13:0x17], pattern[19:23]) != 0 {
		return nil, ErrNotFound
	}

	// TODO: check if it is 64-bit safe
	return uintptr(binary.LittleEndian.Uint32(buf[14+1:])), nil
}

func (p *Patcher) MakeExecPage(code []byte) (uintptr, error) {
	page_size := len(code) + 4095
	page_size -= page_size % 4096
	page, err := p.Syscall(
		syscall.SYS_MMAP,
		0,                  // addr
		uintptr(page_size), // length
		uintptr(syscall.PROT_READ|syscall.PROT_EXEC),  // prot
		uintptr(syscall.MAP_ANON|syscall.MAP_PRIVATE), // flags
		uintptr(0xffffffffffffffff),                   // fd: -1
		0,                                             // offset
	)
	if err != nil {
		return 0, fmt.Errorf("could not allocate the executable scratch page: %w", err)
	}

	err = p.write(page, code)
	if err != nil {
		p.Syscall(
			syscall.SYS_MUNMAP,
			page,
			uintptr(page_size),
		)

		return 0, fmt.Errorf("cannot write to new page: %w", err)
	}

	return page, nil
}

func (p *Patcher) search_data_ref(ptr uintptr, size int, arg interface{}) (interface{}, error) {
	var prologue, prefix, pattern []byte
	var err error

	if p.is64 {
		// prologue = []byte{0x55, 0x48, 0x89, 0xe5} // push rbp; mov rbp, rsp
		prefix = []byte{0}
		pattern = []byte{0x48, 0x8d, 0x3d} // lea rdi, [rip+off32]
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

func (p *Patcher) Shutdown() {
}

func (p *Patcher) read(ptr uintptr, size int) ([]byte, error) {
	buf := make([]byte, size)
	mem, err := os.OpenFile(fmt.Sprintf("/proc/%d/mem", p.pid), os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	defer mem.Close()
	_, err = mem.Seek(int64(ptr), os.SEEK_SET)
	if err != nil {
		log.Printf("read error %x %x seek %s", ptr, size, err)
		return nil, err
	}
	_, err = io.ReadFull(mem, buf)
	if err != nil {
		log.Printf("read error %x %x read %s", ptr, size, err)
		return nil, err
	}

	return buf, nil
}

func (p *Patcher) write(ptr uintptr, buf []byte) error {
	mem, err := os.OpenFile(fmt.Sprintf("/proc/%d/mem", p.pid), os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer mem.Close()
	_, err = mem.Seek(int64(ptr), os.SEEK_SET)
	if err != nil {
		return err
	}
	for len(buf) > 0 {
		n, err := mem.Write(buf)
		if err != nil {
			return err
		}

		buf = buf[n:]
	}

	return nil
}

func (p *Patcher) readU32(ptr uintptr) (uint32, error) {
	buf, err := p.read(ptr, 4)
	if err != nil {
		return 0, err
	}

	return binary.LittleEndian.Uint32(buf[:]), nil
}

func (p *Patcher) readS32(ptr, offset uintptr) (uintptr, error) {
	buf, err := p.read(ptr, 4)
	if err != nil {
		return 0, err
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
		buf, err := p.read(scan+uintptr(len(pattern)), 16)
		if err != nil {
			return nil, err
		}
		if p.readptr(buf, 1) == arg.(uintptr) {
			return p.readptr(buf, 0), nil
		}
	}

	return nil, ErrNotFound
}

func (p *Patcher) SearchFunc(ptr uintptr) (uintptr, error) {
	ptr -= ptr & 0xF
	for i := 0; i < 32; i++ {
		buf, err := p.read(ptr-4, 8)
		if err != nil {
			return 0, err
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
	result, err := p.scan("r--p", p.search_string_cb, []byte(s))
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

		imm32 = 0
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

		err = ps.Patcher.write(ps.result, call_buf[:])
		if err != nil {
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

	result, err := ps.Patcher.scan("r-xp", ps.Patcher.search_data_store, ps.result)
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

	result, err := ps.Patcher.scan("r-xp", ps.Patcher.search_data_ref, ps.result)
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
		Patcher: ps.Patcher,
		String:  "store ref to " + ps.String,
		result:  result.(uintptr),
	}
}

func (ps *PatchState) Call(s string) (err error) {
	if ps.Err != nil {
		return ps.Err
	}

	buf := append([]byte(s), 0)

	err = ps.Patcher.write(ps.Patcher.Scratch, buf)
	if err != nil {
		log.Printf("scratch page: %x", ps.Patcher.Scratch)
		err = fmt.Errorf("cannot write to scratch page: %w", err)
		return
	}

	buf = []byte{
		/*  0 */ 0x48, 0xc7, 0xc7, 0, 0, 0, 0, // [7] mov rdi, imm32
		/*  7 */ 0x48, 0xc7, 0xc0, 0x38, 0, 0, 0, // [7] mov rax, SYS_CLONE
		/* 14 */ 0x48, 0xbe, 0, 0, 0, 0, 0, 0, 0, 0, // [10] mov rsi, stack
		/* 24 */ 0x48, 0x31, 0xd2, // [3] xor rdx, rdx
		/* 27 */ 0x4d, 0x31, 0xd2, // [3] xor r10, r10
		/* 30 */ 0x4d, 0x31, 0xc0, // [3] xor r8, r8
		/* 33 */ 0x0f, 0x05, // [2] syscall
		/* 35 */ 0x48, 0x85, 0xc0, // [3] test rax, rax
		/* 38 */ 0x75, 0x22, // [2] jnz +34
		/* 40 */ 0x48, 0xb9, 0, 0, 0, 0, 0, 0, 0, 0, // [10] mov rcx, imm64
		/* 50 */ 0x48, 0xbf, 0, 0, 0, 0, 0, 0, 0, 0, // [10] mov rdi, imm64
		/* 60 */ 0xff, 0xd1, // [2] call rcx
		// /* 60 */ 0xeb, 0xfe, // [2] jmp $
		// /* 60 */ 0x90, 0x90, // [2] nop
		/* 62 */ 0x48, 0xc7, 0xc0, 0x3c, 0, 0, 0, // [7] mov rax, SYS_EXIT
		/* 69 */ 0x48, 0x31, 0xff, // [3] xor rdi, rdi
		/* 72 */ 0x0f, 0x05, // [2] syscall
		/* 74 */ 0xcc, // [1] int3
	}
	binary.LittleEndian.PutUint32(buf[3:], uint32(syscall.CLONE_THREAD|syscall.CLONE_SIGHAND|syscall.CLONE_VM|syscall.CLONE_VFORK))
	binary.LittleEndian.PutUint64(buf[16:], uint64(ps.Patcher.Stack+STACK_SIZE))
	binary.LittleEndian.PutUint64(buf[42:], uint64(ps.result))
	binary.LittleEndian.PutUint64(buf[52:], uint64(ps.Patcher.Scratch))

	err = ps.Patcher.write(ps.Patcher.ExecScratch, buf)
	if err != nil {
		return err
	}

	_, err = ps.Patcher.Exec(0)
	if err != nil {
		return err
	}

	return nil
}

func (ps *PatchState) LoadRef() *PatchState {
	if ps.Err != nil {
		return ps
	}

	result, err := ps.Patcher.scan("r-xp", ps.Patcher.search_data_load, ps.result)
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

	result, err := ps.Patcher.scan("r-xp", ps.Patcher.search_mul_add, ps.result)
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

	result, err := ps.Patcher.scan("r-xp", ps.Patcher.search_load_arg, &ArgValue{
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

func (p *Patcher) scan(perm_filter string, cb func(ptr uintptr, size int, arg interface{}) (interface{}, error), arg interface{}) (interface{}, error) {
	mapsBuf, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", p.pid))
	if err != nil {
		return nil, err
	}

	maps := strings.Split(string(mapsBuf), "\n")

L:
	for _, line := range maps {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		flds := strings.Fields(line)
		if len(flds) < 5 {
			continue
		}
		if flds[3] == "00:00" {
			break
		}

		for i, c := range perm_filter {
			if c == '-' {
				continue
			}

			if i >= len(flds[1]) || c != rune(flds[1][i]) {
				continue L
			}
		}

		var map_from, map_to uintptr
		_, err = fmt.Sscanf(flds[0], "%x-%x", &map_from, &map_to)
		if err != nil {
			continue
		}

		result, err := cb(map_from, int(map_to-map_from), arg)
		if err == ErrNotFound {
			continue
		} else if err != nil {
			return nil, err
		}

		return result, nil
	}

	return nil, ErrNotFound
}

var ErrNotFound = fmt.Errorf("not found")

// vim: ai:ts=8:sw=8:noet:syntax=go
