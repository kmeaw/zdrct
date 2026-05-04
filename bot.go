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

type Bot struct {
	Balances     map[string]int
	UserName     string
	AdminName    string
	ChannelName  string
	RconClient   *RconClient
	Script       string
	LastBuckets  map[string]time.Time
	Alerter      *Alerter
	Buttons      []*Command
	RewardMap    map[string]*Command
	RewardSet    map[string]bool
	Sound        *Sound
	SoundVolume  int
	TwitchFilter bool

	crediter *time.Ticker
	online   bool

	confDir string

	e *env.Env

	tw_broadcaster *TwitchClient
	tw_bot         *TwitchClient
	yt_bot         *YoutubeClient
	hclient        *http.Client
	ircClient      *irc.Client
	ircConn        net.Conn

	TtsEndpoint string

	mu *sync.Mutex
}

func NewBot(tw_broadcaster, tw_bot *TwitchClient, yt_bot *YoutubeClient, confDir string) *Bot {
	return &Bot{
		Balances:    make(map[string]int),
		LastBuckets: make(map[string]time.Time),
		RewardMap:   make(map[string]*Command),
		RewardSet:   make(map[string]bool),

		tw_broadcaster: tw_broadcaster,
		tw_bot:         tw_bot,
		yt_bot:         yt_bot,

		confDir: confDir,

		hclient: &http.Client{Timeout: 5 * time.Second},

		mu: new(sync.Mutex),
	}
}

func (b *Bot) handleIRCConn() {
	b.mu.Lock()
	client := b.ircClient
	ircConn := b.ircConn
	b.mu.Unlock()

	client.CapRequest("twitch.tv/tags", true)
	client.CapRequest("twitch.tv/membership", true)
	client.CapRequest("twitch.tv/commands", true)
	err := client.Run()
	if err != nil {
		log.Printf("IRC error: %s", err)
		b.mu.Lock()
		b.ircConn = nil
		b.mu.Unlock()
		ircConn.Close()
	}
}

func (b *Bot) Reply(format string, rest ...any) {
	irc_client := b.ircClient
	yt_client := b.yt_bot

	body := fmt.Sprintf(format, rest...)
	if irc_client != nil {
		log.Printf("twitch <- %s", body)
		irc_client.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				"#" + b.ChannelName,
				body,
			},
		})
	}

	if yt_client != nil && yt_client.Token != "" {
		log.Printf("youtube <- %s", body)
		yt_client.SendMessage(context.Background(), body)
	}
}

func (b *Bot) ProcessMessage(ctx context.Context, from, msg string) error {
	flds := strings.Fields(msg)
	if len(flds) == 0 {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.e == nil {
		return fmt.Errorf("script is not loaded, ignoring %q: %q", from, msg)
	}

	if _, ok := b.Balances[strings.ToLower(from)]; !ok {
		b.Balances[strings.ToLower(from)] = 5
	}

	if new_prefix, ok := ctx.Value("prefix").(string); !ok || new_prefix == "" {
		log.Printf("%s: %s", from, msg)
	}

	if strings.HasPrefix(flds[0], "!") {
		prefix := "cmd_"
		if new_prefix, ok := ctx.Value("prefix").(string); ok && new_prefix != "" {
			prefix = new_prefix
		}
		cmd := flds[0][1:]
		if strings.HasPrefix(strings.TrimSpace(msg), "!!") {
			prefix = "cmd" // NOTE: no trailing underscore
			cmd = "bangbang"
		}
		_, err := b.e.Get(prefix + cmd)
		if err != nil {
			if prefix == "cmd_" {
				return fmt.Errorf("Unrecognized command: %q: %s", cmd, err)
			} else {
				return nil
			}
		}

		// TODO: assign a prefix for Twitch rewards
		is_reward := ctx.Value("is_reward")
		if b.TwitchFilter {
			if b.RewardSet[cmd] {
				if is_reward != true {
					return fmt.Errorf("%q can only be called from Twitch rewards", cmd)
				}
			}
		}

		args := make([]string, 0, len(flds)-1)
		if is_reward == true {
			if len(flds) > 1 {
				args = append(args, fmt.Sprintf("%q", strings.Join(flds[1:], " ")))
			}
		} else if prefix == "cmd" && cmd == "bangbang" {
			args = append(args, fmt.Sprintf("%q", strings.TrimPrefix(strings.TrimSpace(msg), "!!")))
		} else {
			for _, arg := range flds[1:] {
				args = append(args, fmt.Sprintf("%q", arg))
			}
		}

		script := fmt.Sprintf("%s%s(%s)", prefix, cmd, strings.Join(args, ", "))
		e := b.e.DeepCopy()
		ctx := context.WithValue(ctx, "from_user", from)

		go func(ctx context.Context, e *env.Env) {
			b.e.Set("eval", func(code string) any {
				result, err := vm.ExecuteContext(ctx, e, nil, code)
				if err != nil {
					log.Printf("error while executing %q: %s", code, err)
					return nil
				}
				return result
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

func (b *Bot) Handle(c *irc.Client, m *irc.Message) {
	var err error

	b.mu.Lock()
	ch := b.ChannelName
	b.mu.Unlock()

	if m.Command == "001" {
		// 001 is a welcome event, so we join channels there
		c.Write("JOIN #" + ch)
	} else if m.Command == "JOIN" && c.FromChannel(m) {
		if strings.ToLower(m.Prefix.User) != strings.ToLower(c.CurrentNick()) {
			err = b.ProcessMessage(
				context.WithValue(context.Background(), "prefix", "event_"),
				m.Prefix.User,
				"!join",
			)
		}
	} else if m.Command == "PART" && c.FromChannel(m) {
		if strings.ToLower(m.Prefix.User) != strings.ToLower(c.CurrentNick()) {
			err = b.ProcessMessage(
				context.WithValue(context.Background(), "prefix", "event_"),
				m.Prefix.User,
				"!part",
			)
		}
	} else if m.Command == "PRIVMSG" && c.FromChannel(m) {
		msg := m.Trailing()
		if m.Prefix == nil {
			log.Printf("bogus message: %#v", m)
			return
		}

		from := m.Prefix.User
		if display_name, ok := m.GetTag("display-name"); ok && strings.ToLower(display_name) == strings.ToLower(from) {
			from = display_name
		}
		if msgid, _ := m.GetTag("msg-id"); msgid == "highlighted-message" {
			msg = "!highlighted " + msg
			err = b.ProcessMessage(context.WithValue(
				context.Background(),
				"prefix",
				"event_",
			), from, msg)
		} else {
			err = b.ProcessMessage(context.Background(), from, msg)
		}
	}
	if err != nil {
		log.Println(err)
	}
}

func (b *Bot) GiveCredits() {
	for range b.crediter.C {
		b.mu.Lock()
		for k := range b.Balances {
			b.Balances[k] += 1
		}
		b.mu.Unlock()
	}
}

func (b *Bot) IsOnline() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.online
}

func (b *Bot) LoadScript(config Config) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	loading := true

	var errors []error

	b.Buttons = nil
	b.RewardMap = map[string]*Command{}
	b.RewardSet = map[string]bool{}
	b.SoundVolume = config.SoundVolume
	b.TwitchFilter = config.NoMappedRewardCommands

	b.e = env.NewEnv()
	_, err := vm.Execute(b.e, nil, `
func from() {
	return "<FAIL>"
}
func is_reward() {
	return false
}
func raid() {
	return new(Raid)
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
		is_reward := ctx.Value("is_reward")
		_, err := orig_is_reward.(func(context.Context) (reflect.Value, reflect.Value))(ctx)
		return reflect.ValueOf(is_reward), err
	}))
	errors = append(errors, b.e.Set("from", func(ctx context.Context) (reflect.Value, reflect.Value) {
		from := ctx.Value("from_user")
		_, err := orig_from.(func(context.Context) (reflect.Value, reflect.Value))(ctx)
		return reflect.ValueOf(from), err
	}))
	errors = append(errors, b.e.Set("raid", func(ctx context.Context) (reflect.Value, reflect.Value) {
		raid := ctx.Value("raid")
		_, err := orig_from.(func(context.Context) (reflect.Value, reflect.Value))(ctx)
		return reflect.ValueOf(raid), err
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

		if b.tw_broadcaster == nil || b.tw_broadcaster.UserID == 0 {
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
		b.RewardSet[command.Cmd] = true
	}))
	errors = append(errors, b.e.Define("admin", b.AdminName))
	errors = append(errors, b.e.Define("balance", func(name string) int {
		b.mu.Lock()
		defer b.mu.Unlock()

		return b.Balances[strings.ToLower(name)]
	}))
	errors = append(errors, b.e.Define("set_balance", func(name string, value int) {
		b.mu.Lock()
		defer b.mu.Unlock()

		b.Balances[strings.ToLower(name)] = value
	}))
	errors = append(errors, b.e.Define("actor_alert", func(actor *Actor, from string) {
		tmpl, err := template.New("actor_alert").Parse(actor.AlertText)
		if err != nil {
			log.Printf("template error: %s", err)
			return
		}
		buf := &bytes.Buffer{}
		err = tmpl.Execute(buf, map[string]any{
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
		b.Alerter.Broadcast(alert, b.SoundVolume)
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
			b.Sound.Play(f.Name(), b.SoundVolume)
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
		err = tmpl.Execute(buf, map[string]any{
			"From":  from,
			"Actor": actor,
		})
		b.Reply("%s", buf.String())
		b.LastBuckets["reply"] = time.Now().Add(time.Second)
	}))
	errors = append(errors, b.e.Define("granum", func(n int, forms ...string) string {
		switch len(forms) {
		case 1:
			return fmt.Sprintf("%d %s", n, forms[0])
		case 2:
			if n == 1 {
				return fmt.Sprintf("%d %s", n, forms[0])
			} else {
				return fmt.Sprintf("%d %s", n, forms[1])
			}
		case 3:
			if (n%100)/10 == 1 {
				return fmt.Sprintf("%d %s", n, forms[2])
			} else if n%10 == 1 {
				return fmt.Sprintf("%d %s", n, forms[0])
			} else if n%10 < 5 {
				return fmt.Sprintf("%d %s", n, forms[1])
			} else {
				return fmt.Sprintf("%d %s", n, forms[2])
			}
		default:
			return fmt.Sprintf("%d", n)
		}
	}))
	errors = append(errors, b.e.Define("shoutout", func(to_id int64) {
		b.mu.Lock()
		defer b.mu.Unlock()

		if b.tw_broadcaster == nil || b.tw_broadcaster.UserID == 0 {
			log.Println("broadcaster token is not set")
			return
		}

		ctx, cancel := context.WithTimeout(
			context.Background(),
			time.Second*10,
		)
		defer cancel()
		err := b.tw_bot.Shoutout(ctx, b.tw_broadcaster.UserID, to_id)
		if err != nil {
			log.Printf("cannot send shoutout: %s", err)
		}
	}))
	errors = append(errors, b.e.Define("reply", func(format string, args ...any) {
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
	errors = append(errors, b.e.Define("sleep", func(duration any) {
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
	errors = append(errors, b.e.Define("float", func(token string) float64 {
		f, err := strconv.ParseFloat(token, 64)
		if err != nil {
			log.Printf("cannot convert %q to float: %s", token, err)
			return 0.0
		}
		return f
	}))
	errors = append(errors, b.e.Define("join", strings.Join))
	errors = append(errors, b.e.Define("rcon", func(format string, args ...any) bool {
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
	errors = append(errors, b.e.Define("debug", func(format string, args ...any) {
		log.Printf("[DEBUG] "+format, args...)
	}))
	errors = append(errors, b.e.Define("rand", func() float64 {
		return rand.Float64()
	}))
	errors = append(errors, b.e.Define("randn", func(n int) int {
		return rand.Intn(n)
	}))
	errors = append(errors, b.e.Define("table", func(name string) *Table {
		table, err := ReadTable(b.confDir, name)
		if err != nil {
			panic(fmt.Errorf("error opening table %q: %w", name, err))
		}
		return table
	}))
	errors = append(errors, b.e.Define("roll", func(args ...any) any {
		if len(args) == 0 {
			return "invalid_args"
		}

		ps := []float64{}
		vs := []any{}
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
			sum += flt
		}
		if len(args)%2 == 1 {
			last := args[len(args)-1]
			ps = append(ps, m)
			vs = append(vs, last)
			sum += m
		}
		roll := rand.Float64()
		log.Printf("Unnormalized ps: %f", ps)
		log.Printf("Roll value is %f", roll)
		for i, p := range ps {
			value := offset + p/sum
			log.Printf("%f .. %f -> %q", offset, value, vs[i])
			if offset <= roll && roll < value {
				log.Printf("%f <= %f and %f < %f", offset, roll, roll, value)
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

		b.Alerter.Broadcast(alert, b.SoundVolume)
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
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	errors = append(errors, b.e.Define("system", func(arg0 string, args ...string) {
		cmd := exec.Command(arg0, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
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
		b.Sound.Play(name, b.SoundVolume)
	}))

	_, err = vm.Execute(b.e, nil, config.Script)
	if err != nil {
		return err
	}

	loading = false

	b.Script = config.Script
	b.TtsEndpoint = config.TtsEndpoint
	return nil
}

func (b *Bot) startIRC() {
	t := time.NewTicker(time.Second * 5)
	defer t.Stop()
	for range t.C {
		b.mu.Lock()
		token := b.tw_bot.Token
		b.mu.Unlock()

		if token == "" {
			continue
		}

		b.mu.Lock()
		b.AdminName = b.tw_broadcaster.Login
		b.UserName = b.tw_bot.Login
		b.ChannelName = b.tw_broadcaster.Login
		b.mu.Unlock()

		log.Println("connecting to Twitch IRC...")

		b.mu.Lock()
		socksProxy := b.tw_bot.SocksProxy
		b.mu.Unlock()

		var ircConn net.Conn
		var err error
		if socksProxy != "" {
			dialer, err := newSOCKS5Dialer(socksProxy, 10*time.Second)
			if err != nil {
				log.Printf("Error creating SOCKS5 dialer: %s", err)
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			conn, err := dialWithProxyContext(ctx, dialer, "tcp", "irc.chat.twitch.tv:6697")
			cancel()
			if err != nil {
				log.Printf("Error dialing Twitch IRC through proxy: %s", err)
				continue
			}
			tlsConn := tls.Client(conn, &tls.Config{ServerName: "irc.chat.twitch.tv"})
			tlsConn.SetDeadline(time.Now().Add(10 * time.Second))
			if err := tlsConn.Handshake(); err != nil {
				conn.Close()
				log.Printf("Error negotiating Twitch IRC TLS through proxy: %s", err)
				continue
			}
			tlsConn.SetDeadline(time.Time{})
			ircConn = tlsConn
		} else {
			dialer := &net.Dialer{Timeout: 10 * time.Second}
			ircConn, err = tls.DialWithDialer(dialer, "tcp", "irc.chat.twitch.tv:6697", nil)
			if err != nil {
				log.Printf("Error dialing Twitch IRC endpoint: %s", err)
				continue
			}
		}

		ircClient := irc.NewClient(ircConn, irc.ClientConfig{
			Nick:    b.UserName,
			Pass:    "oauth:" + token,
			User:    b.UserName,
			Name:    b.UserName,
			Handler: b,
		})

		b.mu.Lock()
		b.ircConn = ircConn
		b.ircClient = ircClient
		b.mu.Unlock()
		b.handleIRCConn()
	}
}

func (b *Bot) ReconnectIRC() {
	b.mu.Lock()
	ircConn := b.ircConn
	b.mu.Unlock()

	if ircConn != nil {
		ircConn.Close()
	}
}

func (b *Bot) startYoutube() {
	t := time.NewTicker(time.Second * 5)
	defer t.Stop()
	for range t.C {
		b.mu.Lock()
		token := b.yt_bot.Token
		b.mu.Unlock()

		if token == "" {
			continue
		}

		if b.yt_bot.LiveChatID == "" {
			continue
		}

		log.Println("connecting to YouTube live chat...")
		ctx := context.Background()

		msgs, err := b.yt_bot.Messages(ctx)
		if err != nil {
			log.Printf("error getting YouTube messages: %s", err)
			continue
		}

		for msg := range msgs {
			b.ProcessMessage(ctx, msg.From, msg.Body)
		}
	}
}

func (b *Bot) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.online {
		return nil
	}

	if b.crediter == nil {
		b.crediter = time.NewTicker(time.Second * 2)
		go b.GiveCredits()
	}

	go b.startIRC()
	go b.startYoutube()

	b.online = true

	return nil
}

func (b *Bot) GetButtons() []*Command {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.Buttons
}

func (b *Bot) GetRewardsMap() map[string]*Command {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.RewardMap
}

// vim: ai:ts=8:sw=8:noet:syntax=go
