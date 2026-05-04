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
	"crypto/tls"
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
	Bot         *Bot
	Config      *RemoteEvent

	ImageCache map[string]string

	conn             *websocket.Conn
	connEventSub     *websocket.Conn
	eventSubLocation *url.URL
	SocksProxy       string
	mu               sync.Mutex
}

func NewRemote(bot *Bot, socksProxy string) *Remote {
	r := &Remote{
		Bot:        bot,
		ImageCache: make(map[string]string),
		SocksProxy: strings.TrimSpace(socksProxy),
		eventSubLocation: &url.URL{
			Scheme: "wss",
			Host:   "eventsub.wss.twitch.tv",
			Path:   "/ws",
		},
	}

	go r.connect()
	go r.watch()

	return r
}

var ErrNoConfig = errors.New("channel name or token are not set")

func websocketAuthority(location *url.URL) string {
	if location.Port() != "" {
		return location.Host
	}

	switch location.Scheme {
	case "ws":
		return net.JoinHostPort(location.Hostname(), "80")
	case "wss":
		return net.JoinHostPort(location.Hostname(), "443")
	default:
		return location.Host
	}
}

func dialWebsocketConfig(config *websocket.Config, socksProxy string, timeout time.Duration) (*websocket.Conn, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	if socksProxy == "" {
		config.Dialer = &net.Dialer{Timeout: timeout}
		return websocket.DialConfig(config)
	}

	dialer, err := newSOCKS5Dialer(socksProxy, timeout)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, err := dialWithProxyContext(ctx, dialer, "tcp", websocketAuthority(config.Location))
	if err != nil {
		return nil, err
	}

	if config.Location.Scheme == "wss" {
		tlsConfig := config.TlsConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{}
		} else {
			tlsConfig = tlsConfig.Clone()
		}
		if tlsConfig.ServerName == "" {
			tlsConfig.ServerName = config.Location.Hostname()
		}

		tlsConn := tls.Client(conn, tlsConfig)
		tlsConn.SetDeadline(time.Now().Add(timeout))
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return nil, err
		}
		tlsConn.SetDeadline(time.Time{})
		conn = tlsConn
	}

	conn.SetDeadline(time.Now().Add(timeout))
	ws, err := websocket.NewClient(config, conn)
	conn.SetDeadline(time.Time{})
	if err != nil {
		conn.Close()
		return nil, err
	}

	return ws, nil
}

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

		Dialer: &net.Dialer{Timeout: 10 * time.Second},

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
	bot := r.Bot
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

		if event.Command == "" {
			continue
		}

		log.Printf("ws: got event: %#v", event)

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

func (r *Remote) watch() {
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

	config := &websocket.Config{
		Location: r.eventSubLocation,

		Origin: &url.URL{
			Scheme: "https",
			Host:   "twitch.tv",
		},

		Version: websocket.ProtocolVersionHybi13,
	}
	r.connEventSub, err = dialWebsocketConfig(config, r.SocksProxy, 10*time.Second)

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
	Payload map[string]any `json:"payload"`
}

type EventSubNotification struct {
	EventSubBase
	Payload struct {
		Subscription struct {
			ID        string         `json:"id"`
			Status    string         `json:"status"`
			Type      string         `json:"type"`
			Version   string         `json:"version"`
			Cost      int            `json:"cost"`
			Condition map[string]any `json:"condition"`
			Transport struct {
				Method    string `json:"method"`
				SessionID string `json:"session_id"`
			} `json:"transport"`
			CreatedAt time.Time `json:"created_at"`
		} `json:"subscription"`
		Event map[string]any `json:"event"`
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

func (r *Remote) processNotification(bot *Bot, event EventSubUnknown, bmsg []byte) error {
	notification_event := EventSubNotification{}
	err := json.Unmarshal(bmsg, &notification_event)
	if err != nil {
		return err
	}

	if bot == nil {
		log.Printf("bot is inactive")
		return nil
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		time.Second*15,
	)
	defer cancel()

	bpayload, _ := json.Marshal(notification_event.Payload.Event)

	switch event.Metadata.SubscriptionType {
	case "channel.raid":
		raid_event := &RaidEvent{}
		err = json.Unmarshal(bpayload, &raid_event)
		if err != nil {
			return fmt.Errorf("eventsub: cannot extract raid event payload from %q: %w", bpayload, err)
		}

		si, err := r.Broadcaster.GetStreamInfo(ctx, raid_event.FromBroadcasterUserID)
		if err != nil {
			log.Printf(
				"eventsub: cannot get stream info for %q: %s",
				raid_event.FromBroadcasterUserLogin,
				err,
			)
			return nil
		}

		raid := &Raid{
			Raider: Raider{
				ID:    raid_event.FromBroadcasterUserID,
				Login: raid_event.FromBroadcasterUserLogin,
				Name:  raid_event.FromBroadcasterUserName,
			},
			Viewers: raid_event.Viewers,
			Stream:  *si,
		}

		name := raid_event.FromBroadcasterUserLogin
		if strings.ToLower(name) == strings.ToLower(raid_event.FromBroadcasterUserName) {
			name = raid_event.FromBroadcasterUserName
		}

		err = bot.ProcessMessage(
			context.WithValue(
				context.WithValue(context.Background(), "raid", raid),
				"prefix", "event_",
			),
			name,
			"!raid",
		)
		if err != nil {
			log.Printf("cannot emit raid event: %s", err)
		}

	case "channel.channel_points_custom_reward_redemption.add":
		r.mu.Lock()
		m := bot.GetRewardsMap()
		r.mu.Unlock()

		redemption := &Redemption{}
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

		name := redemption.UserLogin
		if strings.ToLower(name) == strings.ToLower(redemption.UserName) {
			name = redemption.UserName
		}

		cmd, ok := m[redemption.RewardInfo.ID]
		if ok {
			log.Printf(
				"User %s has redeemed a reward %q: %q",
				name,
				redemption.RewardInfo.Title,
				cmd.Cmd,
			)
			payload := "!" + cmd.Cmd
			if redemption.UserInput != "" {
				payload = payload + " " + redemption.UserInput
			}
			err := bot.ProcessMessage(
				context.WithValue(context.Background(), "is_reward", true),
				name,
				payload,
			)
			if err != nil {
				log.Println(err)
				reward.SetRedemptionStatus(ctx, redemption.ID, "CANCELED")
			} else {
				err = reward.SetRedemptionStatus(ctx, redemption.ID, "FULFILLED")
			}
		} else {
			log.Printf(
				"User %s redeems a reward %q: command is not mapped",
				name,
				reward.ID,
			)

			err = reward.SetRedemptionStatus(ctx, redemption.ID, "CANCELED")
		}
		if err != nil {
			log.Printf("cannot change redemption status: %s", err)
		}
	default:
		log.Printf("eventsub: unexpected message: %s", bmsg)
	}
	return nil
}

func (r *Remote) readEventSub() error {
	r.mu.Lock()
	conn := r.connEventSub
	bot := r.Bot
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

		case "session_reconnect":
			return nil

		case "notification":
			err = r.processNotification(bot, event, bmsg)
			if err != nil {
				return err
			}
		}
	}
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

func (r *Remote) SetSocksProxy(socksProxy string) {
	socksProxy = strings.TrimSpace(socksProxy)

	r.mu.Lock()
	if r.SocksProxy == socksProxy {
		r.mu.Unlock()
		return
	}
	r.SocksProxy = socksProxy
	connEventSub := r.connEventSub
	r.mu.Unlock()

	if connEventSub != nil {
		connEventSub.Close()
	}
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
		req.Header.Set("User-Agent", "github.com/kmeaw/zdrct "+Version)

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
	return enc.Encode(map[string]any{
		"config": map[string]any{
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
