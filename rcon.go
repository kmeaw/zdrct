package main

import (
	"net"
)

type RconClient struct {
	Addr     *net.UDPAddr
	Password string
}

func NewRconClient(hostport, password string) (c *RconClient, err error) {
	c = &RconClient{}
	c.Addr, err = net.ResolveUDPAddr("udp", hostport)
	if err != nil {
		c = nil
		return
	}

	return
}

// vim: ai:ts=8:sw=8:noet:syntax=go
