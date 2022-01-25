package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

type Remote struct {
	Broadcaster *TwitchClient
	IRCBot      *IRCBot
	Config      *RemoteEvent

	ImageCache map[string]string

	conn       *websocket.Conn
	connPubSub *websocket.Conn
	mu         sync.Mutex
}

func NewRemote(bot *IRCBot) *Remote {
	r := &Remote{
		IRCBot:     bot,
		ImageCache: make(map[string]string),
	}

	go r.connect()
	go r.watchRewards()

	return r
}

var ErrNoConfig = errors.New("channel name or token are not set")

func (r *Remote) connectOnce() error {
	var err error

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Broadcaster == nil || r.Broadcaster.Token == "" || r.Broadcaster.Login == "" {
		return ErrNoConfig
	}
	r.conn, err = websocket.DialConfig(&websocket.Config{
		Location: &url.URL{
			Scheme:   "wss",
			Host:     "zd.kmeaw.com",
			Path:     "/channels/" + url.PathEscape(r.Broadcaster.Login) + "/push",
			RawQuery: "auth=" + r.Broadcaster.Token,
		},

		Origin: &url.URL{
			Scheme: "https",
			Host:   "zd.kmeaw.com",
			Path:   "/channels/" + url.PathEscape(r.Broadcaster.Login),
		},

		Dialer: &net.Dialer{
			Timeout: 10 * time.Second,
		},

		Version: websocket.ProtocolVersionHybi13,
	})

	if err != nil {
		return err
	}

	r.sendConfig()

	return nil
}

type RemoteEvent struct {
	Origin   string `json:"origin"`
	Receiver string `json:"receiver"`

	Config struct {
		Buttons []*Command `json:"buttons"`
	} `json:"config,omitempty"`
	Balance  int    `json:"balance,omitempty"`
	Command  string `json:"command,omitempty"`
	IsReward bool   `json:"is_reward,omitempty"`
}

func (r *Remote) readLoop() error {
	r.mu.Lock()
	conn := r.conn
	bot := r.IRCBot
	r.mu.Unlock()

	if conn == nil {
		panic("not connected")
	}

	dec := json.NewDecoder(conn)
	event := RemoteEvent{}
	for {
		err := dec.Decode(&event)
		if err != nil {
			return err
		}

		log.Printf("ws: got event: %#v", event)

		if event.Command == "" {
			continue
		}

		err = bot.ProcessMessage(
			context.WithValue(context.Background(), "is_reward", event.IsReward),
			event.Origin,
			"!"+event.Command,
		)
		if err != nil {
			log.Println(err)
		}
	}
}

func (r *Remote) watchRewards() {
	t := time.NewTicker(time.Second * 10)
	defer t.Stop()
	for {
		err := r.checkRewards()
		if err != nil {
			if err != ErrNoConfig {
				log.Printf("remote: cannot check rewards: %s", err)
			}
		}
		<-t.C
	}
}

func (r *Remote) checkRewards() error {
	r.mu.Lock()
	bot := r.IRCBot
	m := bot.GetRewardsMap()
	broadcaster := r.Broadcaster
	r.mu.Unlock()

	if broadcaster == nil {
		return ErrNoConfig
	}

	rewards := r.Broadcaster.GetRewards()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	for _, reward := range rewards {
		redemptions, err := reward.GetRedemptions(ctx)
		if err != nil {
			return err
		}

		for _, redemption := range redemptions {
			if redemption.Reward == nil {
				log.Printf("unknown reward in redemption: %#v", redemption)
				continue
			}
			cmd, ok := m[redemption.Reward.ID]

			if ok {
				log.Printf(
					"User %s redeems reward %q: %q",
					redemption.UserLogin,
					redemption.Reward.Title,
					cmd.Cmd,
				)
				err := bot.ProcessMessage(
					context.WithValue(context.Background(), "is_reward", true),
					redemption.UserLogin,
					"!"+cmd.Cmd,
				)
				if err != nil {
					log.Println(err)
				} else {
					err = redemption.SetStatus(ctx, "FULFILLED")
				}
			} else {
				log.Printf(
					"User %s redeems reward %q: command is not mapped",
					redemption.UserLogin,
					redemption.Reward.Title,
				)
				err = redemption.SetStatus(ctx, "CANCELED")
			}
			if err != nil {
				log.Printf("cannot change redemition status: %s", err)
			}
		}
		time.Sleep(time.Second)
	}

	return nil
}

func (r *Remote) connect() {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()

	for {
		err := r.connectOnce()
		if err != nil {
			if err != ErrNoConfig {
				log.Printf("ws: connect failed: %s", err)
			}
			<-t.C
			continue
		}

		err = r.readLoop()
		if err != nil {
			log.Printf("ws read error: %s", err)
		}
		r.mu.Lock()
		r.conn.Close()
		r.conn = nil
		r.mu.Unlock()

		<-t.C
	}
}

func (r *Remote) SetBroadcaster(tw *TwitchClient) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Broadcaster = tw
}

func (r *Remote) sendConfig() error {
	if r.Config == nil {
		return nil
	}

	cl := &http.Client{
		Timeout: 10 * time.Second,
	}

	uploaded_buttons := make([]*Command, 0, len(r.Config.Config.Buttons))

	for _, bptr := range r.Config.Config.Buttons {
		button := *bptr
		uploaded_buttons = append(uploaded_buttons, &button)
		if uploaded_path := r.ImageCache[button.Image]; uploaded_path != "" {
			button.Image = uploaded_path
			continue
		}

		if button.Image == "" {
			continue
		}

		asset, ok := Assets[button.Image]
		if !ok {
			log.Printf("error loading button %q: no such asset: %q", button.Cmd, button.Image)
			continue
		}
		img, err := os.ReadFile(asset)
		if err != nil {
			log.Printf("error loading button %q: %s", button.Cmd, err)
			continue
		}

		buf := &bytes.Buffer{}
		digest := sha256.Sum256(img)
		hex_digest := hex.EncodeToString(digest[:])

		mw := multipart.NewWriter(buf)
		w, err := mw.CreateFormField("image")
		if err != nil {
			log.Printf("error creating form field: %s", err)
		}
		w.Write(img)
		if err != nil {
			log.Printf("error writing into multipart form: %s", err)
		}
		mw.Close()

		req, err := http.NewRequest(
			"POST",
			"https://zd.kmeaw.com/buttons/"+hex_digest,
			buf,
		)
		if err != nil {
			panic(err)
		}
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "OAuth "+r.Broadcaster.Token)
		req.Header.Set("User-Agent", "github.com/kmeaw/zdrct")

		resp, err := cl.Do(req)
		if err != nil {
			log.Printf("error uploading button %q: %s", button.Cmd, err)
		}
		dec := json.NewDecoder(resp.Body)
		var result struct {
			Path string `json:"path"`
		}
		err = dec.Decode(&result)
		resp.Body.Close()
		if err != nil {
			log.Printf(
				"error uploading button %q: cannot decode JSON (status code: %d): %s",
				button.Cmd,
				resp.StatusCode,
				err,
			)
			continue
		}
		r.ImageCache[button.Image] = result.Path
		button.Image = result.Path
		log.Printf("image for %q is at %q", button.Cmd, result.Path)
	}

	enc := json.NewEncoder(r.conn)
	return enc.Encode(map[string]interface{}{
		"config": map[string]interface{}{
			"buttons": uploaded_buttons,
		},
	})
}

func (r *Remote) SetConfig(config RemoteEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.Config = &config
	if r.conn != nil {
		r.sendConfig()
	}
}

// vim: ai:ts=8:sw=8:noet:syntax=go
