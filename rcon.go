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
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type RconClient struct {
	Addr     *net.UDPAddr
	Password string

	c *net.UDPConn

	messages <-chan string

	Players                 []string
	PlayerCount, AdminCount int
	Map                     string

	cv *sync.Cond
	mu *sync.Mutex
}

type RconServer struct {
	ctx    context.Context
	cancel context.CancelFunc

	Password string
	error    error

	commands chan string

	c    *net.UDPConn
	peer *net.UDPAddr
	salt []byte
	mu   *sync.Mutex
}

func NewRconServer(password string) (*RconServer, error) {
	ch := make(chan string, 16)
	udpAddr := &net.UDPAddr{
		IP:   net.IP{127, 0, 0, 1},
		Port: 10666,
	}

	r := &RconServer{
		Password: password,
		commands: ch,
	}

	r.ctx, r.cancel = context.WithCancel(context.Background())

	var err error
	r.c, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, err
	}

	r.mu = new(sync.Mutex)

	return r, nil
}

func (r *RconServer) Commands() <-chan string {
	return r.commands
}

func (r *RconServer) Send(s string) error {
	r.mu.Lock()
	peer := r.peer
	c := r.c
	r.mu.Unlock()

	if peer == nil {
		return nil
	}

	_, err := c.WriteToUDP(
		append([]byte{0xff, byte(SVRC_MESSAGE)}, []byte(s)...),
		peer,
	)

	return err
}

func (r *RconServer) Start() {
	buf := make([]byte, 4096)
	c := r.c
	for {
		n, addr, err := c.ReadFromUDP(buf)
		if err != nil {
			if err := r.ctx.Err(); err != nil {
				return
			}

			log.Printf("UDP read error: %s", err)
			time.Sleep(time.Second)
			continue
		}

		data := HuffmanDecode(buf[0:n])
		if len(data) == 0 {
			continue
		}

		switch CLRC(data[0]) {
		case CLRC_BEGINCONNECTION:
			salt := make([]byte, 32)
			_, err := rand.Read(salt)
			if err != nil {
				log.Fatalf("error: cannot get random bytes: %s", err)
			}
			resp := append([]byte{0xff, byte(SVRC_SALT)}, salt...)
			_, err = c.WriteToUDP(resp, addr)
			if err != nil {
				log.Printf("write failed: %s", err)
			} else {
				r.mu.Lock()
				r.salt = salt
				r.mu.Unlock()
			}

		case CLRC_PASSWORD:
			if len(data) != 33 {
				log.Printf("bad CLRC_PASSWORD: %q", data)
				continue
			}

			sum, err := hex.DecodeString(string(data[1:]))
			if err != nil {
				log.Printf("bad CLRC_PASSWORD: %q", data)
				continue
			}

			h := md5.New()
			r.mu.Lock()
			h.Write([]byte(r.salt))
			h.Write([]byte(r.Password))
			r.mu.Unlock()
			sum2 := h.Sum(nil)

			if subtle.ConstantTimeCompare(sum, sum2) != 1 {
				_, err = c.WriteToUDP([]byte{0xff, byte(SVRC_INVALIDPASSWORD)}, addr)
			} else {
				_, err = c.WriteToUDP([]byte{
					0xff,
					byte(SVRC_LOGGEDIN),
					4, // protocol version
					0, // empty hostname
					0, // empty history
				}, addr)

				r.mu.Lock()
				r.peer = addr
				r.mu.Unlock()
			}

			if err != nil {
				log.Printf("write failed: %s", err)
				continue
			}

		case CLRC_DISCONNECT:
			r.mu.Lock()
			r.peer = nil
			r.mu.Unlock()

		case CLRC_COMMAND:
			r.commands <- strings.TrimSpace(string(data[1:]))
		}
	}
}

func (r *RconServer) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.c == nil {
		log.Println("rcon server is already stopped")
	}

	r.c.Close()
	r.c = nil
	close(r.commands)
	r.cancel()
}

// https://wiki.zandronum.com/RCon_protocol

// Messages that the server sends to the client always begin with one
// of the following bytes:
type SVRC byte

const (
	SVRC_OLDPROTOCOL SVRC = iota + 32
	SVRC_BANNED
	SVRC_SALT
	SVRC_LOGGEDIN
	SVRC_INVALIDPASSWORD
	SVRC_MESSAGE
	SVRC_UPDATE
	SVRC_TABCOMPLETE
	SVRC_TOOMANYTABCOMPLETES
)

// Messages that the client sends to the server always begin with one
// of the following bytes:
type CLRC byte

const (
	CLRC_BEGINCONNECTION CLRC = iota + 52
	CLRC_PASSWORD
	CLRC_COMMAND
	CLRC_PONG
	CLRC_DISCONNECT
	CLRC_TABCOMPLETE
)

// Also, when the server sends SVRC_UPDATE, it's immediately followed
// by another byte:
type SVRCU byte

const (
	SVRCU_PLAYERDATA SVRCU = iota
	SVRCU_ADMINCOUNT
	SVRCU_MAP
)

const PROTOCOL_VERSION = 4
const PONG_INTERVAL = time.Second * 5

func NewRconClient() *RconClient {
	r := &RconClient{}
	r.mu = new(sync.Mutex)
	r.cv = sync.NewCond(r.mu)
	r.Addr = &net.UDPAddr{
		IP:   net.IP{127, 0, 0, 1},
		Port: 10666,
	}
	go r.ponger(time.NewTicker(PONG_INTERVAL))

	return r
}

func (r *RconClient) ponger(t *time.Ticker) {
	for range t.C {
		r.mu.Lock()
		conn := r.c
		r.mu.Unlock()

		if conn == nil {
			continue
		}

		_, err := conn.Write([]byte{0xFF, byte(CLRC_PONG)})
		if err != nil {
			log.Printf("cannot pong: %s", err)
		}
	}
}

func (r *RconClient) Recv() ([]byte, error) {
	r.mu.Lock()
	conn := r.c
	r.mu.Unlock()

	if conn == nil {
		return nil, net.ErrClosed
	}

	buf := make([]byte, 4096)
	err := conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	if err != nil {
		return nil, err
	}

	n, err := r.c.Read(buf)
	if err != nil {
		if errors.Is(err, os.ErrDeadlineExceeded) {
			return nil, nil
		}

		return nil, err
	}

	if n == 0 {
		return nil, io.ErrUnexpectedEOF
	}

	return HuffmanDecode(buf[0:n]), nil
}

func (r *RconClient) Send(clrc CLRC, buf []byte) error {
	_, err := r.c.Write(append([]byte{0xff, byte(clrc)}, buf...))
	return err
}

func (r *RconClient) IsOnline() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.c == nil {
		return false
	}

	return true
}

func (r *RconClient) loop(messages chan<- string) {
	for {
		pkt, err := r.Recv()
		if err != nil {
			log.Printf("rcon error: %s", err)
			break
		}

		if pkt == nil {
			continue
		}

		switch SVRC(pkt[0]) {
		case SVRC_MESSAGE:
			messages <- string(pkt[1:])

		case SVRC_UPDATE:
			r.mu.Lock()

			if len(pkt) < 3 {
				log.Printf("bad svrcu: %x", pkt)
				goto bad_svrcu
			}

			switch SVRCU(pkt[1]) {
			case SVRCU_PLAYERDATA:
				r.PlayerCount = int(pkt[2])
				r.Players = strings.Split(string(pkt[3:]), "\000")

			case SVRCU_ADMINCOUNT:
				r.AdminCount = int(pkt[2])

			case SVRCU_MAP:
				r.Map = string(pkt[2:])

			default:
				log.Printf("unexpected svrcu: %x", pkt[1:])
			}

			r.cv.Broadcast()

		bad_svrcu:
			r.mu.Unlock()

		case SVRC_TABCOMPLETE, SVRC_TOOMANYTABCOMPLETES:
			// not implemented yet

		default:
			log.Printf("unexpected pkt: %x", pkt)
		}
	}

	close(messages)
}

func (r *RconClient) Command(cmd string) error {
	return r.Send(CLRC_COMMAND, []byte(cmd))
}

func (r *RconClient) Connect(hostport, password string) (err error) {
	r.mu.Lock()
	prev_addr := r.Addr
	prev_password := r.Password
	r.Password = password
	r.Addr, err = net.ResolveUDPAddr("udp", hostport)
	r.mu.Unlock()

	if err != nil {
		r.mu.Lock()
		r.Addr = prev_addr
		r.Password = prev_password
		r.mu.Unlock()
		return
	}

	r.mu.Lock()
	r.c, err = net.DialUDP("udp", nil, r.Addr)
	r.mu.Unlock()
	if err != nil {
		return
	}

	err = r.Send(CLRC_BEGINCONNECTION, []byte{PROTOCOL_VERSION})
	if err != nil {
		return
	}

	for {
		var pkt []byte
		pkt, err = r.Recv()
		if err != nil {
			return
		}

		if pkt == nil {
			err = fmt.Errorf("timed out")
			r.Close()
			return
		}

		switch SVRC(pkt[0]) {
		case SVRC_LOGGEDIN:
			messages := make(chan string, 16)
			go r.loop(messages)
			r.mu.Lock()
			r.messages = messages
			r.mu.Unlock()

			return

		case SVRC_SALT:
			h := md5.New()
			h.Write(pkt[1:33])
			h.Write([]byte(password))
			hsum := []byte(hex.EncodeToString(h.Sum(nil)))
			err = r.Send(CLRC_PASSWORD, hsum)

		case SVRC_OLDPROTOCOL:
			err = fmt.Errorf("client protocol is too old")

		case SVRC_BANNED:
			err = fmt.Errorf("client is banned")

		case SVRC_INVALIDPASSWORD:
			err = fmt.Errorf("invalid password")

		case SVRC_MESSAGE:
			log.Printf("unsolicited message: %q", pkt[1:])

		default:
			log.Printf("unexpected svrc: %x", pkt)
		}

		if err != nil {
			return
		}
	}
}

func (r *RconClient) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.c == nil {
		return nil
	}

	r.Send(CLRC_DISCONNECT, nil)
	err := r.c.Close()
	r.c = nil

	return err
}

func (r *RconClient) Locked(fn func(*RconClient) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return fn(r)
}

func (r *RconClient) Wait() {
	r.cv.Wait()
}

func (r *RconClient) Messages() <-chan string {
	r.mu.Lock()
	msg := r.messages
	r.mu.Unlock()

	if msg != nil {
		return msg
	}

	fallback := make(chan string)
	close(fallback)

	return fallback
}

// vim: ai:ts=8:sw=8:noet:syntax=go
