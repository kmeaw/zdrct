package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/mattn/anko/env"
	"github.com/mattn/anko/vm"
	"gopkg.in/irc.v3"
)

type IRCBot struct {
	Balances    map[string]int
	UserName    string
	AdminName   string
	ChannelName string
	RconClient  *RconClient
	Script      string
	LastBuckets map[string]time.Time
	Alerter     *Alerter

	crediter *time.Ticker
	online   bool

	e *env.Env

	twitch *TwitchClient
	client *irc.Client
	conn   net.Conn
	mu     *sync.Mutex
}

func NewIRCBot(tw *TwitchClient) *IRCBot {
	return &IRCBot{
		Balances:    make(map[string]int),
		LastBuckets: make(map[string]time.Time),

		twitch: tw,
		mu:     new(sync.Mutex),
	}
}

func (b *IRCBot) handleConn() {
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()

	err := client.Run()
	if err != nil {
		log.Printf("IRC error: %s", err)
		b.mu.Lock()
		b.online = false
		b.conn.Close()
		b.conn = nil
		b.mu.Unlock()
	}
}

func (b *IRCBot) Reply(format string, rest ...interface{}) {
	b.client.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params: []string{
			"#" + b.ChannelName,
			fmt.Sprintf(format, rest...),
		},
	})
}

func (b *IRCBot) Handle(c *irc.Client, m *irc.Message) {
	b.mu.Lock()
	ch := b.ChannelName
	b.mu.Unlock()

	if m.Command == "001" {
		// 001 is a welcome event, so we join channels there
		c.Write("JOIN #" + ch)
	} else if m.Command == "PRIVMSG" && c.FromChannel(m) {
		msg := m.Trailing()
		if m.Prefix == nil {
			log.Printf("bogus message: %#v", m)
			return
		}

		from := m.Prefix.User
		flds := strings.Fields(msg)
		if len(flds) == 0 {
			return
		}

		b.mu.Lock()
		defer b.mu.Unlock()

		if _, ok := b.Balances[from]; !ok {
			b.Balances[from] = 5
		}

		b.e.Set("from", from)

		if strings.HasPrefix(flds[0], "!") {
			cmd := flds[0][1:]
			_, err := b.e.Get("cmd_" + cmd)
			if err != nil {
				log.Printf("Unrecognized command: %q: %s", cmd, err)
				return
			}

			args := make([]string, 0, len(flds)-1)
			for _, arg := range flds[1:] {
				args = append(args, fmt.Sprintf("%q", arg))
			}

			script := fmt.Sprintf("cmd_%s(%s)", cmd, strings.Join(args, ", "))
			_, err = vm.Execute(b.e, nil, script)
			if err != nil {
				log.Printf("cannot execute script %q: %s", script, err)
				return
			}
		}
	}
}

func (b *IRCBot) GiveCredits() {
	for range b.crediter.C {
		b.mu.Lock()
		for k := range b.Balances {
			b.Balances[k] += 1
		}
		b.mu.Unlock()
	}
}

func (b *IRCBot) IsOnline() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.online
}

func (b *IRCBot) LoadScript(script string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.e = env.NewEnv()
	_, err := vm.Execute(b.e, nil, script)
	if err != nil {
		return err
	}

	var errors []error
	errors = append(errors, b.e.Define("from", ""))
	errors = append(errors, b.e.Define("admin", b.AdminName))
	errors = append(errors, b.e.Define("balance", func(name string) int {
		return b.Balances[name]
	}))
	errors = append(errors, b.e.Define("set_balance", func(name string, value int) {
		b.Balances[name] = value
	}))
	errors = append(errors, b.e.Define("reply", func(format string, args ...interface{}) {
		t0 := time.Now()
		t1 := b.LastBuckets["reply"]
		if t0.Before(t1) {
			return
		}
		b.Reply(format, args...)
		b.LastBuckets["reply"] = time.Now().Add(time.Second)
	}))
	errors = append(errors, b.e.Define("last", func(key string) int64 {
		t, ok := b.LastBuckets[key]
		if ok {
			return t.Unix()
		} else {
			return 0
		}
	}))
	errors = append(errors, b.e.Define("set_last", func(key string, delta int64) {
		b.LastBuckets[key] = time.Now().Add(time.Duration(delta) * time.Second)
	}))
	errors = append(errors, b.e.Define("rate", func(key string, delta int64) bool {
		t, ok := b.LastBuckets[key]
		if !ok || time.Now().After(t) {
			b.LastBuckets[key] = time.Now().Add(time.Duration(delta) * time.Second)
			return true
		} else {
			return false
		}
	}))
	errors = append(errors, b.e.Define("sleep", func(duration interface{}) {
		i, ok := duration.(int64)
		if ok {
			time.Sleep(time.Duration(i) * time.Second)
			return
		}

		f, ok := duration.(float64)
		if ok {
			time.Sleep(time.Duration(f) * time.Second)
			return
		}

		log.Printf("Bad argument for sleep: %v (%T)", duration, duration)
	}))
	errors = append(errors, b.e.Define("join", strings.Join))
	errors = append(errors, b.e.Define("rcon", func(cmd string) bool {
		if !b.RconClient.IsOnline() {
			return false
		}

		err := b.RconClient.Command(cmd)
		if err != nil {
			log.Printf("RCON error: %s", err)
			return false
		}

		return true
	}))
	errors = append(errors, b.e.Define("rand", func() float64 {
		return rand.Float64()
	}))
	errors = append(errors, b.e.Define("randn", func(n int) int {
		return rand.Intn(n)
	}))
	errors = append(errors, b.e.Define("roll", func(args ...interface{}) interface{} {
		if len(args) == 0 {
			return "invalid_args"
		}

		ps := []float64{}
		vs := []interface{}{}
		var m, sum, offset float64
		for i := 0; i < len(args)-1; i += 2 {
			flt, ok := args[i].(float64)
			if !ok {
				i, ok := args[i].(int64)
				if !ok {
					return fmt.Sprintf("invalid_p[%T]", args[i])
				} else {
					flt = float64(i)
				}
			}

			ps = append(ps, flt)
			vs = append(vs, args[i+1])
			if flt > m {
				m = flt
			}
			sum += m
		}
		if len(args)%2 == 1 {
			last := args[len(args)-1]
			ps = append(ps, m)
			vs = append(vs, last)
			sum += m
		}
		roll := rand.Float64()
		log.Printf("Roll value is %f", roll)
		for i, p := range ps {
			value := offset + p/sum
			if roll < value {
				return vs[i]
			}
			offset = value
		}
		return vs[len(vs)-1] // should not happen
	}))
	errors = append(errors, b.e.Define("sprintf", fmt.Sprintf))
	errors = append(errors, b.e.Define("alert", func(text string, args ...string) {
		alert := AlertEvent{Text: text}
		switch len(args) {
		case 2:
			alert.Sound = args[1]
			fallthrough
		case 1:
			alert.Image = args[0]
		}
		log.Printf("alert(%q)", text)
		b.Alerter.Broadcast(alert)
	}))
	errors = append(errors, b.e.Define("list_cmds", func() (result []string) {
		for _, line := range strings.Split(b.e.String(), "\n") {
			if strings.HasPrefix(line, "cmd_") {
				kv := strings.SplitN(line, " = ", 2)
				if len(kv) < 2 {
					continue
				}

				result = append(
					result,
					strings.TrimPrefix(kv[0], "cmd_"),
				)
			}
		}

		return
	}))
	for _, err := range errors {
		if err != nil {
			return err
		}
	}

	b.Script = script
	return nil
}

func (b *IRCBot) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.online {
		return nil
	}

	if b.twitch.Token == "" {
		return fmt.Errorf("twitch token is not set")
	}

	if b.crediter == nil {
		b.crediter = time.NewTicker(time.Second * 2)
		go b.GiveCredits()
	}

	conn, err := tls.Dial("tcp", "irc.chat.twitch.tv:6697", nil)
	if err != nil {
		return err
	}

	b.client = irc.NewClient(conn, irc.ClientConfig{
		Nick:    b.UserName,
		Pass:    "oauth:" + b.twitch.Token,
		User:    b.UserName,
		Name:    b.UserName,
		Handler: b,
	})

	b.conn = conn
	go b.handleConn()
	b.online = true

	return nil
}

// vim: ai:ts=8:sw=8:noet:syntax=go
