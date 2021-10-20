package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/context/ctxhttp"
)

const HELIX_REWARDS = "https://api.twitch.tv/helix/channel_points/custom_rewards"
const APP_ID = "ehx3943o3ttw0nplenassv0pme8yvb"
const APP_SCOPES = "channel:manage:redemptions"

type TwitchClient struct {
	Token         string
	BroadcasterID int64
	ExpiresAt     time.Time

	h *http.Client
}

func NewTwitchClient() *TwitchClient {
	return &TwitchClient{
		h: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type APIError struct {
	req  *http.Request
	resp *http.Response
	data []byte
	err  error
}

func (e APIError) Error() string {
	b := &bytes.Buffer{}
	if e.req != nil {
		fmt.Fprintf(b, "error while calling %s: ", e.req.URL)
	}
	if e.resp != nil {
		fmt.Fprintf(b, "got status %d: ", e.resp.StatusCode)
	}
	if e.data != nil {
		fmt.Fprintf(b, "got data: %q: ", string(e.data))
	}
	if e.err != nil {
		b.WriteString(e.err.Error())
	} else {
		b.WriteString("unexpected status code")
	}

	return b.String()
}

func (c *TwitchClient) GetAuthLink(redirect_url, state string) string {
	u := &url.URL{
		Scheme: "https",
		Host:   "id.twitch.tv",
		Path:   "/oauth2/authorize",
	}
	u.RawQuery = (&url.Values{
		"client_id":     []string{APP_ID},
		"redirect_uri":  []string{redirect_url},
		"response_type": []string{"token"},
		"scope":         []string{APP_SCOPES},
		"state":         []string{state},
	}).Encode()

	return u.String()
}

func (c *TwitchClient) apiCall(ctx context.Context, api_url string, payload map[string]interface{}, result interface{}) (err error) {
	var req *http.Request
	var resp *http.Response
	var data []byte

	if c.ExpiresAt.Before(time.Now()) && !c.ExpiresAt.IsZero() {
		return fmt.Errorf("token has expired at %s", c.ExpiresAt)
	}

	if c.BroadcasterID != 0 {
		u, err := url.Parse(api_url)
		if err != nil {
			return err
		}

		qs := u.Query()
		qs.Set("broadcaster_id", strconv.FormatInt(c.BroadcasterID, 10))
	}

	if payload == nil || len(payload) == 0 {
		req, err = http.NewRequest("GET", api_url, nil)
	} else {
		method := "POST"

		if mi, ok := payload["_method"]; ok {
			if ms, ok := mi.(string); ok {
				m := make(map[string]interface{})
				for k := range payload {
					if k == "_method" {
						continue
					}
					m[k] = payload[k]
				}
				payload = m
				method = ms
			}
		}

		data, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("cannot marshal payload: %w", err)
		}

		req, err = http.NewRequest(method, api_url, bytes.NewBuffer(data))
	}

	if err != nil {
		return fmt.Errorf("cannot make http request: %w", err)
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("User-Agent", "github.com/kmeaw/zdrst")
	req.Header.Set("Client-ID", APP_ID)

	resp, err = ctxhttp.Do(ctx, c.h, req)
	if err != nil {
		return APIError{
			req:  req,
			resp: nil,
			data: nil,
			err:  err,
		}
	}

	defer resp.Body.Close()

	data, err = io.ReadAll(resp.Body)
	if err != nil {
		return APIError{
			req:  req,
			resp: nil,
			data: nil,
			err:  err,
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return APIError{
			req:  req,
			resp: resp,
			data: data,
			err:  nil,
		}
	}

	if result == nil {
		return
	}

	err = json.Unmarshal(data, result)
	if err != nil {
		return fmt.Errorf("cannot unmarshal data: %w", err)
	}

	return
}

type twitchValidateReply struct {
	TwitchClientID string   `json:"client_id"`
	Login          string   `json:"login"`
	Scopes         []string `json:"scopes"`
	UserID         int64    `json:"user_id,string"`
	ExpiresIn      int64    `json:"expires_in"`
}

type twitchRewardsReply struct {
	Data []Reward `json:"data"`
}

type RewardImages struct {
	Url1x string `json:"url_1x"`
	Url2x string `json:"url_2x"`
	Url4x string `json:"url_4x"`
}

type Reward struct {
	ID string `json:"id"`

	BroadcasterID    int64  `json:"broadcaster_id,string"`
	BroadcasterLogin string `json:"broadcaster_login"`
	BroadcasterName  string `json:"broadcaster_name"`

	Title               string        `json:"title"`
	Prompt              string        `json:"prompt"`
	Cost                int           `json:"cost"`
	Image               *RewardImages `json:"image"`
	DefaultImage        *RewardImages `json:"image"`
	BackgroundColor     string        `json:"background_color"`
	IsEnabled           bool          `json:"is_enabled"`
	IsUserInputRequired bool          `json:"is_user_input_required"`

	MaxPerStreamSetting struct {
		IsEnabled    bool `json:"is_enabled"`
		MaxPerStream int  `json:"max_per_stream"`
	} `json:"max_per_stream_setting"`
	MaxPerUserPerStreamSetting struct {
		IsEnabled           bool `json:"is_enabled"`
		MaxPerUserPerStream int  `json:"max_per_user_per_stream"`
	} `json:"max_per_user_per_stream_setting"`
	GlobalCooldownSetting struct {
		IsEnabled             bool `json:"is_enabled"`
		GlobalCooldownSeconds int  `json:"global_cooldown_seconds"`
	} `json:"max_per_user_per_stream_setting"`

	IsPaused                          bool       `json:"is_paused"`
	IsInStock                         bool       `json:"is_in_stock"`
	ShouldRedemptionsSkipRequestQueue bool       `json:"should_redemptions_skip_request_queue"`
	RedemptionsRedeemedCurrentStream  *int       `json:"redemptions_redeemed_current_stream"`
	CooldownExpiresAt                 *time.Time `json:"cooldown_expires_at"`
}

func (c *TwitchClient) Prepare(ctx context.Context) error {
	var v twitchValidateReply

	if c.Token == "" {
		return fmt.Errorf("no token")
	}

	err := c.apiCall(
		ctx,
		"https://id.twitch.tv/oauth2/validate",
		nil,
		&v,
	)
	if err != nil {
		return err
	}

	m1 := make(map[string]bool)
	m2 := make(map[string]bool)

	for _, s := range strings.Split(APP_SCOPES, ",") {
		m1[s] = true
	}
	for _, s := range v.Scopes {
		m2[s] = true
	}
	for s := range m1 {
		if !m2[s] {
			return fmt.Errorf("token does not provide scope %q", s)
		}
	}

	c.BroadcasterID = v.UserID
	c.ExpiresAt = time.Now().Add(time.Second * time.Duration(v.ExpiresIn))

	return nil
}

func (c *TwitchClient) Rewards(ctx context.Context) (rs []Reward, err error) {
	var v twitchRewardsReply
	err = c.apiCall(
		ctx,
		"https://api.twitch.tv/helix/channel_points/custom_rewards",
		nil,
		&v,
	)

	return v.Data, err
}

// vim: ai:ts=8:sw=8:noet:syntax=go
