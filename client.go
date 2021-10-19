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

const HELIX_BASE = "https://api.twitch.tv/helix/channel_points/custom_rewards"
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
		data, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("cannot marshal payload: %w", err)
		}

		req, err = http.NewRequest("POST", api_url, bytes.NewBuffer(data))
	}

	if err != nil {
		return fmt.Errorf("cannot make http request: %w", err)
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("User-Agent", "github.com/kmeaw/zdrst")

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

// vim: ai:ts=8:sw=8:noet:syntax=go
