package main

import (
	"log"
	"net"
	"time"
)

type RconClient struct {
	Addr     *net.UDPAddr
	Password string

	c *net.UDPConn
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

func NewRconClient(hostport, password string) (c *RconClient, err error) {
	c = &RconClient{}
	c.Addr, err = net.ResolveUDPAddr("udp", hostport)
	if err != nil {
		c = nil
		return
	}

	return
}

func (r *RconClient) ponger(t *time.Ticker) {
	for range t.C {
		_, err := r.c.Write([]byte{0xFF, byte(CLRC_PONG)})
		if err != nil {
			log.Printf("cannot pong: %s", err)
		}
	}
}

func (r *RconClient) Connect() (err error) {
	r.c, err = net.DialUDP("udp", nil, r.Addr)
	if err != nil {
		return
	}

	defer func() {
		if err != nil && r.c != nil {
			r.c.Close()
			r.c = nil
		}
	} ()

	_, err = r.c.Write([]byte{0xFF, byte(CLRC_BEGINCONNECTION), PROTOCOL_VERSION})
	if err != nil {
		return
	}

	go r.ponger(time.NewTicker(PONG_INTERVAL))

	return
}

func (r *RconClient) readPacket() error {
	return nil
}

func (r *RconClient) Close() (err error) {
	_, err = r.c.Write([]byte{0xFF, byte(CLRC_DISCONNECT)})
	return
}

// vim: ai:ts=8:sw=8:noet:syntax=go
