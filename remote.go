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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
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
	t := time.NewTicker(time.Second * 15)
	defer t.Stop()
	for {
		err := r.checkRewards()
		if err != nil {
			if err != ErrNoConfig {
				log.Printf("pubsub: connect failed: %s", err)
			}
			<-t.C
			continue
		}

		err = r.readPubSub()
		if err != nil {
			log.Printf("pubsub read error: %s", err)
		}
		r.mu.Lock()
		r.connPubSub.Close()
		r.connPubSub = nil
		r.mu.Unlock()

		<-t.C
	}
}

func (r *Remote) checkRewards() error {
	var err error

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Broadcaster == nil || r.Broadcaster.Token == "" || r.Broadcaster.Login == "" {
		return ErrNoConfig
	}
	r.connPubSub, err = websocket.DialConfig(&websocket.Config{
		Location: &url.URL{
			Scheme: "wss",
			Host:   "pubsub-edge.twitch.tv",
		},

		Origin: &url.URL{
			Scheme: "https",
			Host:   "twitch.tv",
		},

		Dialer: &net.Dialer{
			Timeout: 10 * time.Second,
		},

		Version: websocket.ProtocolVersionHybi13,
	})

	if err != nil {
		return err
	}

	enc := json.NewEncoder(r.connPubSub)
	err = enc.Encode(map[string]interface{}{
		"type": "LISTEN",
		"data": map[string]interface{}{
			"topics": []string{
				fmt.Sprintf(
					"channel-points-channel-v1.%d",
					r.Broadcaster.BroadcasterID,
				),
			},
			"auth_token": r.Broadcaster.Token,
		},
	})
	if err != nil {
		return err
	}

	go r.pubSubTicker()
	return nil
}

func (r *Remote) pubSubTicker() {
	r.mu.Lock()
	conn := r.connPubSub
	r.mu.Unlock()

	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for range t.C {
		enc := json.NewEncoder(conn)
		err := enc.Encode(map[string]interface{}{
			"type": "PING",
		})
		if err != nil {
			conn.Close()
			break
		}
	}
}

type PubSubEvent struct {
	Type string `json:"type"`
	Data struct {
		Topic   string `json:"topic"`
		Message string `json:"message"`
	} `json:"data"`
}

type RedemptionMessage struct {
	Type string `json:"type"`
	Data struct {
		Timestamp  time.Time `json:"timestamp"`
		Redemption struct {
			ID   string `json:"id"`
			User struct {
				ID          int64  `json:"id,string"`
				Login       string `json:"login"`
				DisplayName string `json:"display_name"`
			} `json:"user"`
			ChannelID  int64     `json:"channel_id,string"`
			RedeemedAt time.Time `json:"redeemed_at"`
			Reward     struct {
				ID                  string        `json:"id"`
				ChannelID           int64         `json:"channel_id,string"`
				Title               string        `json:"title"`
				Prompt              string        `json:"prompt,omitempty"`
				Cost                int           `json:"cost"`
				IsUserInputRequired bool          `json:"is_user_input_required"`
				IsSubOnly           bool          `json:"is_sub_only"`
				Image               *RewardImages `json:"image"`
				DefaultImage        *RewardImages `json:"default_image"`
				BackgroundColor     string        `json:"background_color"`
				IsEnabled           bool          `json:"is_enabled"`
				IsPaused            bool          `json:"is_paused"`
				IsInStock           bool          `json:"is_in_stock"`
				MaxPerStream        struct {
					IsEnabled    bool `json:"is_enabled"`
					MaxPerStream int  `json:"max_per_stream"`
				} `json:"max_per_stream"`
				ShouldRedemptionsSkipRequestQueue bool        `json:"should_redemptions_skip_request_queue"`
				TemplateID                        interface{} `json:"template_id"`
				MaxPerUserPerStreamSetting        struct {
					IsEnabled           bool `json:"is_enabled"`
					MaxPerUserPerStream int  `json:"max_per_user_per_stream"`
				} `json:"max_per_user_per_stream"`
				GlobalCooldownSetting struct {
					IsEnabled      bool `json:"is_enabled"`
					GlobalCooldown int  `json:"global_cooldown_seconds"`
				} `json:"global_cooldown"`
				CooldownExpiresAt *time.Time `json:"cooldown_expires_at"`
			} `json:"reward"`
			Status    string `json:"status"`
			UserInput string `json:"user_input,omitempty"`
		} `json:"redemption"`
	} `json:"data"`
}

func (r *Remote) readPubSub() error {
	r.mu.Lock()
	conn := r.connPubSub
	bot := r.IRCBot
	r.mu.Unlock()

	if conn == nil {
		panic("not connected")
	}

	dec := json.NewDecoder(conn)
	event := PubSubEvent{}
	for {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		err := dec.Decode(&event)
		if err != nil {
			return err
		}

		if event.Type == "PONG" {
			continue
		} else if event.Type == "RECONNECT" {
			break
		} else if event.Type == "MESSAGE" {
			// ok
		} else if event.Type == "RESPONSE" {
		} else {
			log.Printf("pubsub: got unexpected type: %q", event.Type)
			continue
		}

		if !strings.HasPrefix(event.Data.Topic, "channel-points-channel-v1.") {
			continue
		}

		var rmsg RedemptionMessage
		err = json.Unmarshal([]byte(event.Data.Message), &rmsg)
		if err != nil {
			log.Printf("unmarshal failed: %s", err)
			continue
		}
		redemption := rmsg.Data.Redemption

		r.mu.Lock()
		m := bot.GetRewardsMap()
		r.mu.Unlock()

		ctx, cancel := context.WithTimeout(
			context.Background(),
			time.Second*15,
		)
		defer cancel()

		reward := &Reward{
			RewardCore: RewardCore{
				ID:                                redemption.Reward.ID,
				Title:                             redemption.Reward.Title,
				Prompt:                            redemption.Reward.Prompt,
				Cost:                              redemption.Reward.Cost,
				BackgroundColor:                   redemption.Reward.BackgroundColor,
				IsEnabled:                         redemption.Reward.IsEnabled,
				IsUserInputRequired:               redemption.Reward.IsUserInputRequired,
				IsPaused:                          redemption.Reward.IsPaused,
				ShouldRedemptionsSkipRequestQueue: redemption.Reward.ShouldRedemptionsSkipRequestQueue,
			},

			Image:        redemption.Reward.Image,
			DefaultImage: redemption.Reward.DefaultImage,
			IsInStock:    redemption.Reward.IsInStock,
		}

		r.mu.Lock()
		reward.SetClient(r.Broadcaster)
		r.mu.Unlock()

		cmd, ok := m[redemption.Reward.ID]
		if ok {
			log.Printf(
				"User %s redeems reward %q: %q",
				redemption.User.Login,
				redemption.Reward.Title,
				cmd.Cmd,
			)
			err := bot.ProcessMessage(
				context.WithValue(context.Background(), "is_reward", true),
				redemption.User.Login,
				"!"+cmd.Cmd,
			)
			if err != nil {
				log.Println(err)
			} else {
				err = reward.SetRedemptionStatus(ctx, redemption.ID, "FULFILLED")
			}
		} else {
			log.Printf(
				"User %s redeems reward %q: command is not mapped",
				redemption.User.Login,
				reward.ID,
			)
			err = reward.SetRedemptionStatus(ctx, redemption.ID, "CANCELED")
		}
		if err != nil {
			log.Printf("cannot change redemption status: %s", err)
		}
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
