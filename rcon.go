package main

import (
	"crypto/md5"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

type RconClient struct {
	Addr     *net.UDPAddr
	Password string

	c  *net.UDPConn
	mu *sync.Mutex

	Messages <-chan string
	Updates  <-chan Update
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

type Update struct {
	SVRCU                   SVRCU
	Players                 []string
	PlayerCount, AdminCount int
	Map                     string
	Data                    []byte
}

const PROTOCOL_VERSION = 4
const PONG_INTERVAL = time.Second * 5

func NewRconClient() *RconClient {
	r := &RconClient{
		mu: &sync.Mutex{},
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
	buf := make([]byte, 4096)
	err := r.c.SetReadDeadline(time.Now().Add(4 * time.Second))
	if err != nil {
		return nil, err
	}

	n, err := r.c.Read(buf)
	if err != nil {
		return nil, err
	}

	if n == 0 {
		return nil, io.ErrUnexpectedEOF
	}

	return HuffmanDecode(buf[0:n]), nil
}

func (r *RconClient) Send(clrc CLRC, buf []byte) error {
	_, err := r.c.Write(append([]byte{byte(clrc)}, buf...))
	return err
}

func (r *RconClient) loop(messages chan<- string, updates chan<- Update) {
	for {
		pkt, err := r.Recv()
		if err != nil {
			log.Printf("rcon error: %s", err)
			break
		}

		switch SVRC(pkt[0]) {
		case SVRC_MESSAGE:
			messages <- string(pkt[1:])

		case SVRC_UPDATE:
			upd := Update{
				SVRCU: SVRCU(pkt[1]),
				Data:  pkt[2:],
			}
			switch upd.SVRCU {
			case SVRCU_PLAYERDATA:
				upd.PlayerCount = int(pkt[3])
				upd.Players = strings.Split(string(pkt[4:]), "\000")

			case SVRCU_ADMINCOUNT:
				upd.AdminCount = int(pkt[3])

			case SVRCU_MAP:
				upd.Map = string(upd.Data)

			default:
				log.Printf("unexpected svrcu: %x", pkt[1:])
			}

			updates <- upd

		case SVRC_TABCOMPLETE, SVRC_TOOMANYTABCOMPLETES:
			// not implemented yet

		default:
			log.Printf("unexpected pkt: %x", pkt)
		}
	}

	close(messages)
	close(updates)
}

func (r *RconClient) Command(cmd string) error {
	return r.Send(CLRC_COMMAND, []byte(cmd))
}

func (r *RconClient) Connect(hostport, password string) (err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Addr, err = net.ResolveUDPAddr("udp", hostport)
	if err != nil {
		r.Addr = nil
		return
	}

	r.c, err = net.DialUDP("udp", nil, r.Addr)
	if err != nil {
		return
	}

	defer func() {
		if err != nil && r.c != nil {
			r.c.Close()
			r.c = nil
		}
	}()

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

		switch SVRC(pkt[0]) {
		case SVRC_LOGGEDIN:
			messages := make(chan string, 16)
			updates := make(chan Update, 16)

			go r.loop(messages, updates)

			r.Messages = messages
			r.Updates = updates

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

	return
}

func (r *RconClient) Close() error {
	return r.Send(CLRC_DISCONNECT, nil)
}

// vim: ai:ts=8:sw=8:noet:syntax=go
