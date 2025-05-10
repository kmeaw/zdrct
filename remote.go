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
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

type Remote struct {
	Broadcaster *TwitchClient
	IRCBot      *IRCBot
	Config      *RemoteEvent

	ImageCache map[string]string

	conn             *websocket.Conn
	connEventSub     *websocket.Conn
	eventSubLocation *url.URL
	mu               sync.Mutex
}

func NewRemote(bot *IRCBot) *Remote {
	r := &Remote{
		IRCBot:     bot,
		ImageCache: make(map[string]string),
		eventSubLocation: &url.URL{
			Scheme: "wss",
			Host:   "eventsub.wss.twitch.tv",
			Path:   "/ws",
		},
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

func (r *Remote) writeLoop() error {
	r.mu.Lock()
	conn := r.conn
	r.mu.Unlock()

	t := time.NewTicker(15 * time.Second)
	defer t.Stop()

	for range t.C {
		_, err := conn.Write([]byte("{}"))
		if err != nil {
			log.Printf("write loop failed: %s", err)
			return err
		}
	}
	return nil
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
	t := time.NewTicker(time.Second * 1)
	defer t.Stop()
	for {
		err := r.connectEventSub()
		if err != nil {
			if err != ErrNoConfig {
				log.Printf("eventsub: connect failed: %s", err)
			}
			<-t.C
			continue
		}

		log.Printf("connected to eventsub")

		err = r.readEventSub()
		if err != nil {
			log.Printf("eventsub read error: %s", err)
		}
		r.mu.Lock()
		r.connEventSub.Close()
		r.connEventSub = nil
		r.mu.Unlock()

		if err != nil {
			<-t.C
		}
	}
}

func (r *Remote) connectEventSub() error {
	var err error

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.Broadcaster == nil || r.Broadcaster.Token == "" || r.Broadcaster.Login == "" {
		return ErrNoConfig
	}
	r.connEventSub, err = websocket.DialConfig(&websocket.Config{
		Location: r.eventSubLocation,

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

	return nil
}

type EventSubMetadata struct {
	MessageID        string    `json:"message_id"`
	MessageType      string    `json:"message_type"`
	MessageTimestamp time.Time `json:"message_timestamp"`
	SubscriptionType string    `json:"subscription_type,omitempty"`
}

type EventSubBase struct {
	Metadata EventSubMetadata `json:"metadata"`
}

type EventSubUnknown struct {
	EventSubBase
	Payload map[string]interface{} `json:"payload"`
}

type EventSubNotification struct {
	EventSubBase
	Payload struct {
		Subscription struct {
			ID        string                 `json:"id"`
			Status    string                 `json:"status"`
			Type      string                 `json:"type"`
			Version   string                 `json:"version"`
			Cost      int                    `json:"cost"`
			Condition map[string]interface{} `json:"condition"`
			Transport struct {
				Method    string `json:"method"`
				SessionID string `json:"session_id"`
			} `json:"transport"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"subscription"`
		Event map[string]interface{} `json:"event"`
	} `json:"payload"`
}

type EventSubReconnect struct {
	EventSubBase
	Session struct {
		Id                      string    `json:"id"`
		Status                  string    `json:"status"`
		KeepaliveTimeoutSeconds int       `json:"keepalive_timeout_seconds"`
		ReconnectURL            string    `json:"reconnect_url"`
		ConnectedAt             time.Time `json:"connected_at"`
	} `json:"session"`
}

type EventSubWelcome struct {
	EventSubBase
	Payload struct {
		Session struct {
			ID                      string    `json:"id"`
			Status                  string    `json:"status"`
			ConnectedAt             time.Time `json:"connected_at"`
			KeepaliveTimeoutSeconds int       `json:"keepalive_timeout_seconds"`
			ReconnectURL            string    `json:"reconnect_url"`
		} `json:"session"`
	} `json:"payload"`
}

func (r *Remote) readEventSub() error {
	r.mu.Lock()
	conn := r.connEventSub
	bot := r.IRCBot
	r.mu.Unlock()

	if conn == nil {
		panic("not connected")
	}

	dec := json.NewDecoder(conn)
	event := EventSubUnknown{}
	timeout := 60
	for {
		conn.SetReadDeadline(time.Now().Add(time.Duration(timeout) * time.Second))
		err := dec.Decode(&event)
		if err != nil {
			return err
		}

		bmsg, _ := json.Marshal(event)

		switch event.Metadata.MessageType {
		case "session_keepalive":
			// just reset the deadline
		case "session_welcome":
			welcome_event := EventSubWelcome{}
			err = json.Unmarshal(bmsg, &welcome_event)
			if err != nil {
				return err
			}

			if s := welcome_event.Payload.Session.ReconnectURL; s != "" {
				r.mu.Lock()
				r.eventSubLocation, err = url.Parse(s)
				r.mu.Unlock()
				if err != nil {
					return fmt.Errorf("cannot parse reconnect_url: %q: %w", s, err)
				}
			}

			if t := welcome_event.Payload.Session.KeepaliveTimeoutSeconds; 15 < t && t < 300 {
				timeout = t
			}
			r.mu.Lock()
			err = r.Broadcaster.Subscribe(context.Background(), welcome_event.Payload.Session.ID)
			r.mu.Unlock()
			if err != nil {
				return err
			}

			log.Printf("session %s has been subscribed to rewards", welcome_event.Payload.Session.ID)

		case "session_reconnect":
			return nil

		case "notification":
			if event.Metadata.SubscriptionType != "channel.channel_points_custom_reward_redemption.add" {
				log.Printf("eventsub: unexpected message: %s", bmsg)
				continue
			}

			notification_event := EventSubNotification{}
			err = json.Unmarshal(bmsg, &notification_event)
			if err != nil {
				return err
			}

			if bot == nil {
				log.Printf("bot is inactive")
				continue
			}

			r.mu.Lock()
			m := bot.GetRewardsMap()
			r.mu.Unlock()

			ctx, cancel := context.WithTimeout(
				context.Background(),
				time.Second*15,
			)
			defer cancel()

			redemption := &Redemption{}
			bpayload, _ := json.Marshal(notification_event.Payload.Event)
			err = json.Unmarshal(bpayload, &redemption)
			if err != nil {
				return fmt.Errorf("eventsub: cannot extract redemption payload from %q: %w", bpayload, err)
			}

			reward := &Reward{
				RewardCore: RewardCore{
					ID:     redemption.RewardInfo.ID,
					Title:  redemption.RewardInfo.Title,
					Prompt: redemption.RewardInfo.Prompt,
					Cost:   redemption.RewardInfo.Cost,
				},
			}

			r.mu.Lock()
			reward.SetClient(r.Broadcaster)
			r.mu.Unlock()

			cmd, ok := m[redemption.RewardInfo.ID]
			if ok {
				log.Printf(
					"User %s has redeemed a reward %q: %q",
					redemption.UserLogin,
					redemption.RewardInfo.Title,
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
					err = reward.SetRedemptionStatus(ctx, redemption.ID, "FULFILLED")
				}
			} else {
				log.Printf(
					"User %s redeems a reward %q: command is not mapped",
					redemption.UserLogin,
					reward.ID,
				)

				err = reward.SetRedemptionStatus(ctx, redemption.ID, "CANCELED")
			}
			if err != nil {
				log.Printf("cannot change redemption status: %s", err)
			}
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

		go r.writeLoop()

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
