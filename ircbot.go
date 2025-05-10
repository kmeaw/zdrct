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
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/mattn/anko/env"
	"github.com/mattn/anko/vm"
	"gopkg.in/irc.v3"
)

type Actor struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AlertText  string `json:"alert_text,omitempty"`
	AlertImage string `json:"alert_image,omitempty"`
	AlertSound string `json:"alert_sound,omitempty"`
	Reply      string `json:"reply,omitempty"`
}

type Command struct {
	Cmd   string `json:"cmd"`
	Text  string `json:"text"`
	Image string `json:"image"`
}

type IRCBot struct {
	Balances    map[string]int
	UserName    string
	AdminName   string
	ChannelName string
	RconClient  *RconClient
	Script      string
	LastBuckets map[string]time.Time
	Alerter     *Alerter
	Buttons     []*Command
	RewardMap   map[string]*Command
	Sound       *Sound

	crediter *time.Ticker
	online   bool

	e *env.Env

	tw_broadcaster *TwitchClient
	tw_bot         *TwitchClient
	TtsEndpoint    string

	client  *irc.Client
	hclient *http.Client

	conn net.Conn
	mu   *sync.Mutex
}

func NewIRCBot(tw_broadcaster, tw_bot *TwitchClient) *IRCBot {
	return &IRCBot{
		Balances:    make(map[string]int),
		LastBuckets: make(map[string]time.Time),
		RewardMap:   make(map[string]*Command),

		tw_broadcaster: tw_broadcaster,
		tw_bot:         tw_bot,

		hclient: &http.Client{Timeout: 5 * time.Second},

		mu: new(sync.Mutex),
	}
}

func (b *IRCBot) handleConn() {
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()

	client.CapRequest("twitch.tv/tags", true)
	client.CapRequest("twitch.tv/membership", true)
	client.CapRequest("twitch.tv/commands", true)
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

func (b *IRCBot) ProcessMessage(ctx context.Context, from, msg string) error {
	flds := strings.Fields(msg)
	if len(flds) == 0 {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.e == nil {
		return fmt.Errorf("script is not loaded, ignoring %q: %q", from, msg)
	}

	if _, ok := b.Balances[from]; !ok {
		b.Balances[from] = 5
	}

	if strings.HasPrefix(flds[0], "!") {
		cmd := flds[0][1:]
		if strings.HasPrefix(flds[0], "!!") {
			cmd = flds[0][2:]
		}
		_, err := b.e.Get("cmd_" + cmd)
		if err != nil {
			return fmt.Errorf("Unrecognized command: %q: %s", cmd, err)
		}

		args := make([]string, 0, len(flds)-1)
		if strings.HasPrefix(flds[0], "!!") {
			args = append(args, fmt.Sprintf("%q", strings.TrimSpace(msg[len(flds[0]):])))
		} else {
			for _, arg := range flds[1:] {
				args = append(args, fmt.Sprintf("%q", arg))
			}
		}

		script := fmt.Sprintf("cmd_%s(%s)", cmd, strings.Join(args, ", "))
		e := b.e.DeepCopy()
		ctx := context.WithValue(ctx, "from_user", from)

		go func(ctx context.Context, e *env.Env) {
			b.e.Set("eval", func(code string) interface{} {
				result, err := vm.ExecuteContext(ctx, e, nil, code)
				if err != nil {
					log.Printf("error while executing %q: %s", code, err)
					return nil
				}
				return result
			})
			b.e.Set("forth", func(tokens ...string) (interface{}, error) {
				b.mu.Lock()
				e := b.e.DeepCopy()
				b.mu.Unlock()

				return b.EvalForth(ctx, e, tokens...)
			})

			_, err = vm.ExecuteContext(ctx, e, nil, script)
			if err != nil {
				log.Printf("cannot execute script %q: %s", script, err)
				return
			}
		}(ctx, e)
	}

	return nil
}

func (b *IRCBot) Handle(c *irc.Client, m *irc.Message) {
	var err error

	b.mu.Lock()
	ch := b.ChannelName
	b.mu.Unlock()

	if m.Command == "001" {
		// 001 is a welcome event, so we join channels there
		c.Write("JOIN #" + ch)
	} else if m.Command == "JOIN" && c.FromChannel(m) {
		if strings.ToLower(m.Prefix.User) != strings.ToLower(c.CurrentNick()) {
			err = b.ProcessMessage(context.Background(), m.Prefix.User, "!event_join")
		}
	} else if m.Command == "PART" && c.FromChannel(m) {
		if strings.ToLower(m.Prefix.User) != strings.ToLower(c.CurrentNick()) {
			err = b.ProcessMessage(context.Background(), m.Prefix.User, "!event_part")
		}
	} else if m.Command == "PRIVMSG" && c.FromChannel(m) {
		msg := m.Trailing()
		if m.Prefix == nil {
			log.Printf("bogus message: %#v", m)
			return
		}

		from := m.Prefix.User
		if msgid, _ := m.GetTag("msg-id"); msgid == "highlighted-message" {
			msg = "!!event_highlighted " + msg
		}
		err = b.ProcessMessage(context.Background(), from, msg)
	}
	if err != nil {
		log.Println(err)
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

	loading := true

	var errors []error

	b.Buttons = nil
	b.RewardMap = map[string]*Command{}

	b.e = env.NewEnv()
	_, err := vm.Execute(b.e, nil, `
func from() {
	return "<FAIL>"
}
func is_reward() {
	return false
}
	`)
	if err != nil {
		return err
	}

	errors = append(errors, b.e.DefineType("Command", Command{}))
	errors = append(errors, b.e.DefineType("Reward", Reward{}))
	errors = append(errors, b.e.DefineType("Actor", Actor{}))
	orig_is_reward, err := b.e.Get("is_reward")
	orig_from, err := b.e.Get("from")
	errors = append(errors, err)
	errors = append(errors, b.e.Set("is_reward", func(ctx context.Context) (reflect.Value, reflect.Value) {
		from := ctx.Value("is_reward")
		_, err := orig_is_reward.(func(context.Context) (reflect.Value, reflect.Value))(ctx)
		return reflect.ValueOf(from), err
	}))
	errors = append(errors, b.e.Set("from", func(ctx context.Context) (reflect.Value, reflect.Value) {
		from := ctx.Value("from_user")
		_, err := orig_from.(func(context.Context) (reflect.Value, reflect.Value))(ctx)
		return reflect.ValueOf(from), err
	}))
	errors = append(errors, b.e.Define("add_command", func(commands ...*Command) {
		if !loading {
			log.Println("dynamic add_command is not allowed")
			return
		}

		for _, command := range commands {
			b.Buttons = append(b.Buttons, command)
		}
	}))
	errors = append(errors, b.e.Define("map_reward", func(reward *Reward, command *Command) {
		if !loading {
			log.Println("dynamic add_reward is not allowed")
			return
		}

		ctx, cancel := context.WithTimeout(
			context.Background(),
			time.Second*10,
		)
		defer cancel()
		m := map[string]*Reward{}

		if b.tw_broadcaster == nil || b.tw_broadcaster.BroadcasterID == 0 {
			log.Println("broadcaster token is not set")
			return
		}
		for _, reward := range b.tw_broadcaster.Rewards {
			m[reward.Key()] = reward
		}

		reward.SetClient(b.tw_broadcaster)

		if lr, ok := m[reward.Key()]; ok {
			if lr.Prompt != reward.Prompt || lr.Cost != reward.Cost {
				lr.Prompt = reward.Prompt
				if reward.Cost != 0 {
					lr.Cost = reward.Cost
				}
				err := lr.Save(ctx)
				if err != nil {
					log.Printf("error updating reward: %s", err)
				}
			}
			reward.ID = lr.ID
		} else {
			err := b.tw_broadcaster.CreateReward(ctx, reward)
			if err != nil {
				log.Printf("error creating reward: %s", err)
			}
		}

		b.RewardMap[reward.ID] = command
	}))
	errors = append(errors, b.e.Define("admin", b.AdminName))
	errors = append(errors, b.e.Define("balance", func(name string) int {
		b.mu.Lock()
		defer b.mu.Unlock()

		return b.Balances[name]
	}))
	errors = append(errors, b.e.Define("set_balance", func(name string, value int) {
		b.mu.Lock()
		defer b.mu.Unlock()

		b.Balances[name] = value
	}))
	errors = append(errors, b.e.Define("actor_alert", func(actor *Actor, from string) {
		tmpl, err := template.New("actor_alert").Parse(actor.AlertText)
		if err != nil {
			log.Printf("template error: %s", err)
			return
		}
		buf := &bytes.Buffer{}
		err = tmpl.Execute(buf, map[string]interface{}{
			"From":  from,
			"Actor": actor,
		})

		b.mu.Lock()
		defer b.mu.Unlock()

		alert := AlertEvent{
			Text:  buf.String(),
			Image: actor.AlertImage,
			Sound: actor.AlertSound,
		}

		log.Printf("alert(%q)", alert.Text)
		b.Alerter.Broadcast(alert)
	}))
	errors = append(errors, b.e.Define("tts", func(msg string) bool {
		b.mu.Lock()
		defer b.mu.Unlock()

		if b.TtsEndpoint == "" {
			log.Println("TTS endpoint is not set.")
			return false
		}

		u, err := url.Parse(b.TtsEndpoint)
		if err != nil {
			log.Printf("Invalid TTS endpoint: %s", err)
			return false
		}

		ui := u.User
		qs := u.Query()

		u.User = nil
		u.RawQuery = ""

		for k, vv := range qs {
			if len(vv) == 1 && vv[0] == "TEXT" {
				qs[k] = []string{msg}
			}
		}

		req, err := http.NewRequest("POST", u.String(), strings.NewReader(qs.Encode()))
		if err != nil {
			log.Printf("TTS cannot construct a request: %s", err)
			return false
		}

		if ui != nil {
			pass, ok := ui.Password()
			if ok {
				req.Header.Set("Authorization", ui.Username()+" "+pass)
			}
		}

		req.Header.Set("User-Agent", "https://github.com/kmeaw/zdrct")

		resp, err := b.hclient.Do(req)
		if err != nil {
			log.Printf("TTS request has failed: %s", err)
			return false
		}

		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("TTS request has failed: unexpected status: %d (%q)", err, body)
			return false
		}

		f, err := os.CreateTemp("", "zdrct-tts.*.mp3")
		if err != nil {
			log.Printf("Cannot create a temporary file: %s", err)
			return false
		}

		_, err = io.Copy(f, resp.Body)
		if err != nil {
			log.Printf("Cannot write into the temporary file %q: %s", f.Name(), err)
			return false
		}

		go func() {
			b.Sound.Play(f.Name())
			time.Sleep(time.Second) // FIXME: ugly hack for windows
			os.Remove(f.Name())
		}()

		return true
	}))
	errors = append(errors, b.e.Define("actor_reply", func(actor *Actor, from string) {
		b.mu.Lock()
		defer b.mu.Unlock()

		t0 := time.Now()
		t1 := b.LastBuckets["reply"]
		if t0.Before(t1) {
			return
		}
		tmpl, err := template.New("actor_reply").Parse(actor.Reply)
		if err != nil {
			log.Printf("template error: %s", err)
			return
		}
		buf := &bytes.Buffer{}
		err = tmpl.Execute(buf, map[string]interface{}{
			"From":  from,
			"Actor": actor,
		})
		b.Reply("%s", buf.String())
		b.LastBuckets["reply"] = time.Now().Add(time.Second)
	}))
	errors = append(errors, b.e.Define("reply", func(format string, args ...interface{}) {
		b.mu.Lock()
		defer b.mu.Unlock()

		t0 := time.Now()
		t1 := b.LastBuckets["reply"]
		if t0.Before(t1) {
			return
		}
		b.Reply(format, args...)
		b.LastBuckets["reply"] = time.Now().Add(time.Second)
	}))
	errors = append(errors, b.e.Define("last", func(key string) int64 {
		b.mu.Lock()
		defer b.mu.Unlock()

		t, ok := b.LastBuckets[key]
		if ok {
			return t.Unix()
		} else {
			return 0
		}
	}))
	errors = append(errors, b.e.Define("set_last", func(key string, delta int64) {
		b.mu.Lock()
		defer b.mu.Unlock()

		b.LastBuckets[key] = time.Now().Add(time.Duration(delta) * time.Second)
	}))
	errors = append(errors, b.e.Define("rate", func(key string, delta int64) bool {
		b.mu.Lock()
		defer b.mu.Unlock()

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
	errors = append(errors, b.e.Define("int", func(token string) int64 {
		n, err := strconv.ParseInt(token, 0, 64)
		if err != nil {
			log.Printf("cannot convert %q to int: %s", token, err)
			return -1
		}
		return n
	}))
	errors = append(errors, b.e.Define("join", strings.Join))
	errors = append(errors, b.e.Define("rcon", func(format string, args ...interface{}) bool {
		b.mu.Lock()
		defer b.mu.Unlock()

		if !b.RconClient.IsOnline() {
			return false
		}

		err := b.RconClient.Command(fmt.Sprintf(format, args...))
		if err != nil {
			log.Printf("RCON error: %s", err)
			return false
		}

		return true
	}))
	errors = append(errors, b.e.Define("debug", func(format string, args ...interface{}) {
		log.Printf("[DEBUG] "+format, args...)
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

		b.mu.Lock()
		defer b.mu.Unlock()

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
	errors = append(errors, b.e.Define("eval", nil))
	errors = append(errors, b.e.Define("forth", nil))
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	errors = append(errors, b.e.Define("system", func(arg0 string, args ...string) {
		cmd := exec.Command(arg0, args...)
		err := cmd.Start()
		if err != nil {
			log.Printf("cannot start %q: %s", arg0, err)
			return
		}
		go func(cmd *exec.Cmd) {
			if err := cmd.Wait(); err != nil {
				log.Printf("error while waiting for %q to complete: %s", arg0, err)
			}
		}(cmd)
	}))
	errors = append(errors, b.e.Define("play", func(name string) {
		b.Sound.Play(name)
	}))

	_, err = vm.Execute(b.e, nil, script)
	if err != nil {
		return err
	}

	loading = false

	b.Script = script
	return nil
}

func (b *IRCBot) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.online {
		return nil
	}

	if b.tw_bot.Token == "" {
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
		Pass:    "oauth:" + b.tw_bot.Token,
		User:    b.UserName,
		Name:    b.UserName,
		Handler: b,
	})

	b.conn = conn
	go b.handleConn()
	b.online = true

	return nil
}

func (b *IRCBot) GetButtons() []*Command {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.Buttons
}

func (b *IRCBot) GetRewardsMap() map[string]*Command {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.RewardMap
}

// vim: ai:ts=8:sw=8:noet:syntax=go
