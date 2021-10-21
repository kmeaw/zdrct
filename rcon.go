package main

import (
	"crypto/md5"
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

	Messages <-chan string

	Players                 []string
	PlayerCount, AdminCount int
	Map                     string

	cv *sync.Cond
	mu *sync.Mutex
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
		return nil, nil
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
			log.Printf("rcon msg: %q", pkt[1:])
			messages <- string(pkt[1:])

		case SVRC_UPDATE:
			r.mu.Lock()

			switch SVRCU(pkt[1]) {
			case SVRCU_PLAYERDATA:
				r.PlayerCount = int(pkt[3])
				r.Players = strings.Split(string(pkt[4:]), "\000")

			case SVRCU_ADMINCOUNT:
				r.AdminCount = int(pkt[3])

			case SVRCU_MAP:
				r.Map = string(pkt[3:])

			default:
				log.Printf("unexpected svrcu: %x", pkt[1:])
			}

			r.cv.Broadcast()
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
	r.Addr, err = net.ResolveUDPAddr("udp", hostport)
	r.mu.Unlock()

	if err != nil {
		r.Addr = prev_addr
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
			r.Messages = messages

			return

		case SVRC_SALT:
			h := md5.New()
			h.Write(pkt[1:33])
			h.Write([]byte(r.Password))
			err = r.Send(CLRC_PASSWORD, []byte(fmt.Sprintf("%x", h.Sum(nil))))

		case SVRC_OLDPROTOCOL:
			err = fmt.Errorf("client protocol is too old")

		case SVRC_BANNED:
			err = fmt.Errorf("client is banned")

		case SVRC_INVALIDPASSWORD:
			err = fmt.Errorf("invalid password")

		case SVRC_MESSAGE:
			log.Printf("unsolicited message: %q", pkt[1:])
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

// vim: ai:ts=8:sw=8:noet:syntax=go
