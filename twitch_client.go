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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context/ctxhttp"
)

const HELIX_REWARDS = "https://api.twitch.tv/helix/channel_points/custom_rewards"
const APP_ID = "ehx3943o3ttw0nplenassv0pme8yvb"
const DEFAULT_APP_SCOPES = "chat:read,chat:edit,channel:read:redemptions,channel:manage:redemptions"
const DEFAULT_BOT_SCOPES = "chat:read,chat:edit"

var ErrNoRewards = fmt.Errorf("no rewards")

type TwitchClient struct {
	Token         string
	Login         string
	BroadcasterID int64
	ExpiresAt     time.Time
	Scopes        []string
	Purpose       string
	Rewards       []*Reward
	rewardCache   map[string]*Reward

	h  *http.Client
	mu *sync.Mutex
}

type TwitchClientOpts struct {
	Scopes  string
	Purpose string
}

func NewTwitchClient(opts TwitchClientOpts) *TwitchClient {
	return &TwitchClient{
		h: &http.Client{
			Timeout: 10 * time.Second,
		},
		rewardCache: make(map[string]*Reward),
		mu:          new(sync.Mutex),
		Scopes:      strings.Split(opts.Scopes, ","),
		Purpose:     opts.Purpose,
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
		fmt.Fprintf(b, "error while calling %s %s: ", e.req.Method, e.req.URL)
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

func (c *TwitchClient) GetAuthLink(redirect_url, csrf_token string) string {
	u := &url.URL{
		Scheme: "https",
		Host:   "id.twitch.tv",
		Path:   "/oauth2/authorize",
	}
	v := &url.Values{}
	v.Set("csrf_token", csrf_token)
	v.Set("purpose", c.Purpose)
	u.RawQuery = (&url.Values{
		"client_id":     []string{APP_ID},
		"redirect_uri":  []string{redirect_url},
		"response_type": []string{"token"},
		"scope":         []string{strings.Join(c.Scopes, " ")},
		"state":         []string{v.Encode()},
		"force_verify":  []string{"true"},
	}).Encode()

	return u.String()
}

func (c *TwitchClient) apiCall(ctx context.Context, api_url string, payload interface{}, result interface{}) (err error) {
	var req *http.Request
	var resp *http.Response
	var data []byte

	if c.ExpiresAt.Before(time.Now()) && !c.ExpiresAt.IsZero() {
		c.Token = ""
		return fmt.Errorf("token has expired at %s", c.ExpiresAt)
	}

	u, err := url.Parse(api_url)
	if err != nil {
		return err
	}
	qs := u.Query()
	if c.BroadcasterID != 0 {

		qs.Set("broadcaster_id", strconv.FormatInt(c.BroadcasterID, 10))
		u.RawQuery = qs.Encode()
		api_url = u.String()
	}

	if payload == nil {
		req, err = http.NewRequest("GET", api_url, nil)
	} else {
		method := "POST"

		if plm, ok := payload.(map[string]interface{}); ok {
			if mi, ok := plm["_method"]; ok {
				if ms, ok := mi.(string); ok {
					m := make(map[string]interface{})
					for k := range plm {
						if k == "_method" {
							continue
						}
						m[k] = plm[k]
					}
					payload = m
					method = ms
				}
			}
			if mi, ok := plm["_payload"]; ok {
				if ms, ok := mi.(string); ok {
					payload = ms
				}
			}
		}

		if method == "DELETE" || method == "GET" {
			plm, ok := payload.(map[string]interface{})
			if !ok {
				return fmt.Errorf("unexpected type: %T, expecting map[string]interface{}", payload)
			}
			for k, v := range plm {
				qs.Set(k, fmt.Sprintf("%s", v))
			}
			u.RawQuery = qs.Encode()
			api_url = u.String()
			req, err = http.NewRequest(method, api_url, nil)
		} else {
			data, err = json.Marshal(payload)
			if err != nil {
				return fmt.Errorf("cannot marshal payload: %w", err)
			}

			req, err = http.NewRequest(method, api_url, bytes.NewBuffer(data))
			req.Header.Set("Content-Type", "application/json")
		}
	}

	if err != nil {
		return fmt.Errorf("cannot make http request: %w", err)
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("User-Agent", "github.com/kmeaw/zdrct")
	req.Header.Set("Client-ID", APP_ID)
	req.Header.Set("Accept", "*/*")

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

	if result != nil {
		err = json.Unmarshal(data, result)
		if err != nil {
			return fmt.Errorf("cannot unmarshal data: %w", err)
		}
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
	Data []*Reward `json:"data"`
}

type twitchRedemptionsReply struct {
	Data []*Redemption `json:"data"`
}

type RewardImages struct {
	Url1x string `json:"url_1x"`
	Url2x string `json:"url_2x"`
	Url4x string `json:"url_4x"`
}

type RewardCore struct {
	ID string `json:"id,omitempty"`

	Title                             string `json:"title" form:"title"`
	Prompt                            string `json:"prompt" form:"prompt"`
	Cost                              int    `json:"cost" form:"cost,string"`
	BackgroundColor                   string `json:"background_color"`
	IsEnabled                         bool   `json:"is_enabled"`
	IsUserInputRequired               bool   `json:"is_user_input_required"`
	IsPaused                          bool   `json:"is_paused"`
	ShouldRedemptionsSkipRequestQueue bool   `json:"should_redemptions_skip_request_queue"`
}

type CreateRewardRequest struct {
	RewardCore

	IsMaxPerStreamEnabled        bool `json:"is_max_per_stream_enabled"`
	MaxPerStream                 int  `json:"max_per_stream,omitempty"`
	IsMaxPerStreamPerUserEnabled bool `json:"is_max_per_user_per_stream_enabled"`
	MaxPerUserPerStream          int  `json:"max_per_user_per_stream,omitempty"`
	IsGlobalCooldownEnabled      bool `json:"is_global_cooldown_enabled"`
	GlobalCooldownSeconds        int  `json:"global_cooldown_seconds,omitempty"`
}

type Reward struct {
	RewardCore
	tw *TwitchClient

	BroadcasterID    int64  `json:"broadcaster_id,string,omitempty"`
	BroadcasterLogin string `json:"broadcaster_login,omitempty"`
	BroadcasterName  string `json:"broadcaster_name,omitempty"`

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
	} `json:"global_cooldown_setting"`

	Image        *RewardImages `json:"image"`
	DefaultImage *RewardImages `json:"default_image"`

	IsInStock                        bool       `json:"is_in_stock"`
	RedemptionsRedeemedCurrentStream *int       `json:"redemptions_redeemed_current_stream"`
	CooldownExpiresAt                *time.Time `json:"cooldown_expires_at"`
}

type Redemption struct {
	Reward *Reward

	BroadcasterName  string `json:"broadcaster_name"`
	BroadcasterLogin string `json:"broadcaster_login"`
	BroadcasterID    int64  `json:"broadcaster_id,string"`

	ID string `json:"id"`

	UserName  string `json:"user_name"`
	UserLogin string `json:"user_login"`
	UserID    int64  `json:"user_id,string"`

	UserInput  string    `json:"user_input,omitempty"`
	Status     string    `json:"status"`
	RedeemedAt time.Time `json:"redeemed_at"`
	RewardInfo struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Prompt string `json:"prompt,omitempty"`
		Cost   int    `json:"cost"`
	} `json:"reward"`
}

func (r Reward) Key() string {
	if r.Title == "" {
		return strconv.Itoa(r.Cost)
	} else {
		return r.Title
	}
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

	for _, s := range c.Scopes {
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

	c.mu.Lock()
	defer c.mu.Unlock()

	c.BroadcasterID = v.UserID
	c.ExpiresAt = time.Now().Add(time.Second * time.Duration(v.ExpiresIn))
	c.Login = v.Login

	return err
}

func (c *TwitchClient) LoadRewards(ctx context.Context) error {
	var v twitchRewardsReply
	err := c.apiCall(
		ctx,
		"https://api.twitch.tv/helix/channel_points/custom_rewards",
		nil,
		&v,
	)

	if err != nil {
		return err
	}

	if v.Data == nil {
		return ErrNoRewards
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.Rewards = v.Data
	for _, r := range c.Rewards {
		c.rewardCache[r.ID] = r
		r.tw = c
	}
	return nil
}

func (r Reward) ToCreateRequest() CreateRewardRequest {
	return CreateRewardRequest{
		RewardCore:                   r.RewardCore,
		IsMaxPerStreamEnabled:        r.MaxPerStreamSetting.IsEnabled,
		MaxPerStream:                 r.MaxPerStreamSetting.MaxPerStream,
		IsMaxPerStreamPerUserEnabled: r.MaxPerUserPerStreamSetting.IsEnabled,
		MaxPerUserPerStream:          r.MaxPerUserPerStreamSetting.MaxPerUserPerStream,
		IsGlobalCooldownEnabled:      r.GlobalCooldownSetting.IsEnabled,
		GlobalCooldownSeconds:        r.GlobalCooldownSetting.GlobalCooldownSeconds,
	}
}

func (c *TwitchClient) CreateReward(ctx context.Context, reward *Reward) error {
	reward_req := reward.ToCreateRequest()
	var v twitchRewardsReply
	err := c.apiCall(
		ctx,
		"https://api.twitch.tv/helix/channel_points/custom_rewards",
		reward_req,
		&v,
	)

	if err != nil {
		return err
	}

	if v.Data == nil || len(v.Data) == 0 {
		return ErrNoRewards
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	reward.ID = v.Data[0].ID
	c.rewardCache[reward.ID] = reward
	c.Rewards = append(c.Rewards, reward)
	return nil
}

func (r *Reward) SetClient(tw *TwitchClient) {
	r.tw = tw
}

func (r *Reward) Save(ctx context.Context) error {
	u, err := url.Parse(
		"https://api.twitch.tv/helix/channel_points/custom_rewards",
	)
	if err != nil {
		return err
	}
	qs := u.Query()
	qs.Set("id", r.ID)
	u.RawQuery = qs.Encode()
	reward_req := r.ToCreateRequest()

	err = r.tw.apiCall(
		ctx,
		u.String(),
		map[string]interface{}{
			"_method":  "PATCH",
			"_payload": reward_req,
		},
		nil,
	)

	if err != nil {
		return err
	}

	return nil
}

func (c *TwitchClient) DeleteReward(ctx context.Context, reward Reward) error {
	err := c.apiCall(
		ctx,
		"https://api.twitch.tv/helix/channel_points/custom_rewards",
		map[string]interface{}{
			"_method": "DELETE",
			"id":      reward.ID,
		},
		nil,
	)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for i, r := range c.Rewards {
		if r.ID == reward.ID {
			c.Rewards = append(c.Rewards[:i], c.Rewards[i+1:]...)
			return nil
		}
	}

	return nil
}

func (r *Reward) GetRedemptions(ctx context.Context) ([]*Redemption, error) {
	var v twitchRedemptionsReply
	err := r.tw.apiCall(
		ctx,
		"https://api.twitch.tv/helix/channel_points/custom_rewards/redemptions",
		map[string]interface{}{
			"_method":   "GET",
			"reward_id": r.ID,
			"status":    "UNFULFILLED",
		},
		&v,
	)

	if err != nil {
		return nil, err
	}

	r.tw.mu.Lock()
	defer r.tw.mu.Unlock()

	for _, redemption := range v.Data {
		redemption.Reward = r.tw.rewardCache[redemption.RewardInfo.ID]
	}

	return v.Data, nil
}

func (r *Reward) SetRedemptionStatus(ctx context.Context, redemption_id, new_status string) error {
	u, err := url.Parse(
		"https://api.twitch.tv/helix/channel_points/custom_rewards/redemptions",
	)
	if err != nil {
		return err
	}

	qs := u.Query()
	qs.Set("id", redemption_id)
	qs.Set("reward_id", r.ID)
	u.RawQuery = qs.Encode()

	var v twitchRewardsReply
	err = r.tw.apiCall(
		ctx,
		u.String(),
		map[string]interface{}{
			"_method": "PATCH",
			"status":  new_status,
		},
		&v,
	)

	if err != nil {
		return err
	}

	return nil
}

func (r *Redemption) SetStatus(ctx context.Context, new_status string) error {
	err := r.Reward.SetRedemptionStatus(ctx, r.ID, new_status)
	if err != nil {
		return err
	}

	r.Status = new_status
	return nil
}

func (c *TwitchClient) GetRewards() []*Reward {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.Rewards
}

// vim: ai:ts=8:sw=8:noet:syntax=go
