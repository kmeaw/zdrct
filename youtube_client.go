/**
 * Copyright 2025 kmeaw
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
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/go-querystring/query"
	"golang.org/x/net/context/ctxhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	streamlist "zdrct/youtube.api"
)

const yt_CLIENT_ID = "372706359110-1vmetuoh9bp6b7funr2len7o0gfonpeu.apps.googleusercontent.com"
const yt_CLIENT_SECRET = "\107\117\103\123\120\130\055\110\153\157\161\170\070\104\102\141\115\066\101\113\126\132\123\061\144\107\124\150\170\172\145\151\101\124\066"
const yt_BOT_SCOPES = "https://www.googleapis.com/auth/youtube"
const yt_SERVER_ADDR = "youtube.googleapis.com:443"

type YoutubeClient struct {
	h          *http.Client
	mu         *sync.Mutex
	socksProxy string
	streamConn *grpc.ClientConn

	Login        string
	Token        string
	RefreshToken string
	ExpiresAt    time.Time
	LiveChatID   string
}

type YoutubeClientOpts struct {
	SocksProxy string
}

type YoutubeMessage struct {
	From string
	Body string
}

func NewYoutubeClient(opts YoutubeClientOpts) *YoutubeClient {
	return &YoutubeClient{
		h:          newHTTPClient(opts.SocksProxy, 10*time.Second),
		mu:         new(sync.Mutex),
		socksProxy: strings.TrimSpace(opts.SocksProxy),
	}
}

func (c *YoutubeClient) SetSocksProxy(socksProxy string) {
	socksProxy = strings.TrimSpace(socksProxy)
	c.mu.Lock()
	defer c.mu.Unlock()

	c.socksProxy = socksProxy
	c.h = newHTTPClient(socksProxy, 10*time.Second)
}

func (c *YoutubeClient) ReconnectLiveChat() {
	c.mu.Lock()
	streamConn := c.streamConn
	c.mu.Unlock()

	if streamConn != nil {
		streamConn.Close()
	}
}

var ytNonce int = 0

func (c *YoutubeClient) GetAuthLink(redirect_url, csrf_token string) string {
	ytNonce += 1
	u := &url.URL{
		Scheme: "https",
		Host:   "accounts.google.com",
		Path:   "/o/oauth2/v2/auth",
	}
	v := &url.Values{}
	v.Set("csrf_token", csrf_token)
	digest := sha256.Sum256([]byte(csrf_token))
	u.RawQuery = (&url.Values{
		"client_id":     []string{yt_CLIENT_ID},
		"redirect_uri":  []string{redirect_url},
		"response_type": []string{"code"},
		"nonce":         []string{fmt.Sprintf("%x-%d", digest, ytNonce)},
		"prompt":        []string{"consent"},
		"access_type":   []string{"offline"},
		"scope":         []string{yt_BOT_SCOPES},
		"state":         []string{v.Encode()},
	}).Encode()

	return u.String()
}

type liveBroadcastItem struct {
	Id      string `json:"id"`
	Snippet struct {
		Title      string `json:"title"`
		LiveChatId string `json:"liveChatId"`
	} `json:"snippet"`
}

type liveBroadcasts struct {
	Items []liveBroadcastItem `json:"items"`
}

func (c *YoutubeClient) apiCall(ctx context.Context, api_url string, payload any, result any) (err error) {
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

	if payload == nil {
		req, err = http.NewRequest("GET", api_url, nil)
	} else {
		method := "POST"

		if plm, ok := payload.(map[string]any); ok {
			if mi, ok := plm["_method"]; ok {
				if ms, ok := mi.(string); ok {
					m := make(map[string]any)
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
			plm, ok := payload.(map[string]any)
			if !ok {
				return fmt.Errorf("unexpected type: %T, expecting map[string]any", payload)
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
			return fmt.Errorf("cannot unmarshal data: %q: %w", data, err)
		}
	}

	return
}

type youtubeValidateReply struct {
	YoutubeClientID string `json:"aud"`
	Login           string `json:"email"`
	Scopes          string `json:"scope"`
	UserID          int64  `json:"user_id,string"`
	ExpiresIn       int64  `json:"expires_in,string"`
}

func (c *YoutubeClient) Prepare(ctx context.Context) error {
	var v youtubeValidateReply

	if c.Token == "" {
		if c.RefreshToken != "" {
			c.Refresh()
		}
		if c.Token == "" {
			return fmt.Errorf("no token")
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.ExpiresAt = time.Now().Add(time.Second * 15)
	err := c.apiCall(
		ctx,
		"https://oauth2.googleapis.com/tokeninfo",
		nil,
		&v,
	)
	if err != nil {
		return err
	}

	if !strings.Contains(v.Scopes, "https://www.googleapis.com/auth/youtube") {
		return fmt.Errorf("token does not provide youtube scope")
	}

	c.ExpiresAt = time.Now().Add(time.Second * time.Duration(v.ExpiresIn))
	c.Login = v.Login

	return err
}

type authCodeRequest struct {
	Code         string `url:"code,omitempty"`
	ClientID     string `url:"client_id"`
	ClientSecret string `url:"client_secret"`
	RefreshToken string `url:"refresh_token,omitempty"`
	RedirectUri  string `url:"redirect_uri,omitempty"`
	GrantType    string `url:"grant_type"`
}

type authCodeResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int64  `json:"expires_in"`
	IDToken      string `json:"id_token"`
	Scopes       string `json:"scope"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
}

func (c *YoutubeClient) AuthCode(ctx context.Context, redirect_uri string, code string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	areq, err := query.Values(authCodeRequest{
		Code:         code,
		ClientID:     yt_CLIENT_ID,
		ClientSecret: yt_CLIENT_SECRET,
		RedirectUri:  redirect_uri,
		GrantType:    "authorization_code",
	})
	if err != nil {
		log.Panicf("cannot create auth request: %s", err)
	}

	req, err := http.NewRequest("POST", "https://oauth2.googleapis.com/token", bytes.NewBufferString(areq.Encode()))
	if err != nil {
		log.Panicf("cannot create request: %s", err)
	}
	req.Header.Set("User-Agent", "github.com/kmeaw/zdrct")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := ctxhttp.Do(ctx, c.h, req)
	if err != nil {
		return fmt.Errorf("cannot exchange code for token: %w", err)
	}

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
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

	var aresp authCodeResponse
	err = json.Unmarshal(data, &aresp)
	if err != nil {
		return fmt.Errorf("cannot exchange code for token: unmarshal: %w", err)
	}

	c.Token = aresp.AccessToken
	if aresp.RefreshToken != "" {
		c.RefreshToken = aresp.RefreshToken
	}
	c.ExpiresAt = time.Now().Add(time.Second * time.Duration(aresp.ExpiresIn))

	return nil
}

type youtubeSearchRequest struct {
	Part      string `url:"part"` // id, liveStreamingDetails
	ID        string `url:"id,omitempty"`
	ChannelID string `url:"channelId,omitempty"`
	EventType string `url:"eventType,omitempty"` // live
	Type      string `url:"type,omitempty"`      // video
}

type youtubeSearchResponse struct {
	Items []struct {
		Kind string `json:"kind"`
		ID   struct {
			Kind    string `json:"kind"`
			VideoID string `json:"videoId"`
		} `json:"id"`
	} `json:"items"`
}

type youtubeVideoResponse struct {
	Items []struct {
		Kind                 string `json:"kind"`
		ID                   string `json:"id"`
		LiveStreamingDetails struct {
			ActiveLiveChatId string `json:"activeLiveChatId"`
		} `json:"liveStreamingDetails"`
	} `json:"items"`
}

func (c *YoutubeClient) ResolveChannel(channel_id string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	vreq, err := query.Values(youtubeSearchRequest{
		Part:      "id",
		ChannelID: channel_id,
		EventType: "live",
		Type:      "video",
	})
	if err != nil {
		log.Panicf("cannot serialize youtube search query: %s", err)
	}

	var sresp youtubeSearchResponse
	err = c.apiCall(ctx, "https://www.googleapis.com/youtube/v3/search?"+vreq.Encode(), nil, &sresp)
	if err != nil {
		return "", fmt.Errorf("cannot resolve channel %q to video: %w", channel_id, err)
	}

	var vid string
	if len(sresp.Items) == 1 {
		item := sresp.Items[0]

		if item.Kind != "youtube#searchResult" {
			return "", fmt.Errorf("unexpected search result kind for channel %q: %q", channel_id, item.Kind)
		}

		if item.ID.Kind != "youtube#video" {
			return "", fmt.Errorf("unexpected item ID kind for channel %q: %q", channel_id, item.ID.Kind)
		}

		vid = item.ID.VideoID
	}

	if vid == "" {
		return "", fmt.Errorf("no live video found for %q", channel_id)
	}

	vreq, err = query.Values(youtubeSearchRequest{
		Part: "liveStreamingDetails",
		ID:   vid,
	})
	if err != nil {
		log.Panicf("cannot serialize youtube video info query: %s", err)
	}

	var vresp youtubeVideoResponse
	err = c.apiCall(ctx, "https://www.googleapis.com/youtube/v3/videos?"+vreq.Encode(), nil, &vresp)
	if err != nil {
		return "", fmt.Errorf("cannot fetch video info for %q: %w", vid, err)
	}

	var lcid string
	if len(vresp.Items) == 1 {
		item := vresp.Items[0]

		if item.Kind != "youtube#video" {
			return "", fmt.Errorf("unexpected video kind for video %q: %q", vid, item.Kind)
		}

		lcid = item.LiveStreamingDetails.ActiveLiveChatId
	}

	if lcid == "" {
		return "", fmt.Errorf("could not get live streaming details for video %q", vid)
	}

	return lcid, nil
}

func (c *YoutubeClient) GetAPI(ctx context.Context, method string, kv map[string]string) (map[string]any, error) {
	if c.Token == "" {
		return nil, fmt.Errorf("no token")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	u, err := url.Parse("https://www.googleapis.com/youtube/v3/" + method)
	if err != nil {
		return nil, err
	}

	m := map[string]any{
		"_method": "GET",
	}
	for k := range kv {
		m[k] = kv[k]
	}
	v := map[string]any{}
	err = c.apiCall(
		ctx,
		u.String(),
		m,
		&v,
	)
	if err != nil {
		return nil, err
	}

	return v, nil
}

type youtubeLiveChatTextMessageDetails struct {
	MessageText string `json:"messageText"`
}

type youtubeLiveChatSnippet struct {
	LiveChatID         string                            `json:"liveChatId"`
	Type               string                            `json:"type"` // "textMessageEvent"
	TextMessageDetails youtubeLiveChatTextMessageDetails `json:"textMessageDetails"`
}

type youtubeLiveChatInsertReq struct {
	Snippet youtubeLiveChatSnippet `json:"snippet"`
}

func (c *YoutubeClient) SendMessage(ctx context.Context, message string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	insertReq := youtubeLiveChatInsertReq{
		Snippet: youtubeLiveChatSnippet{
			LiveChatID: c.LiveChatID,
			Type:       "textMessageEvent",
			TextMessageDetails: youtubeLiveChatTextMessageDetails{
				MessageText: message,
			},
		},
	}

	err := c.apiCall(
		ctx,
		"https://www.googleapis.com/youtube/v3/liveChat/messages?part=snippet",
		insertReq,
		nil,
	)
	if err != nil {
		return fmt.Errorf("cannot send youtube reply: %w", err)
	}

	return nil
}

func (c *YoutubeClient) Refresh() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.RefreshToken == "" {
		return
	}

	log.Println("Refreshing YouTube token...")

	areq, err := query.Values(authCodeRequest{
		ClientID:     yt_CLIENT_ID,
		ClientSecret: yt_CLIENT_SECRET,
		RefreshToken: c.RefreshToken,
		GrantType:    "refresh_token",
	})
	if err != nil {
		log.Panicf("cannot create auth request: %s", err)
	}

	req, err := http.NewRequest("POST", "https://oauth2.googleapis.com/token", bytes.NewBufferString(areq.Encode()))
	if err != nil {
		log.Panicf("cannot create request: %s", err)
	}
	req.Header.Set("User-Agent", "github.com/kmeaw/zdrct")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	resp, err := ctxhttp.Do(ctx, c.h, req)
	if err != nil {
		log.Printf("cannot exchange code for token: %s", err)
		return
	}

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("error reading refresh response: %s", err)
		return
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		log.Printf("expected status during refresh: %d", resp.StatusCode)
		return
	}

	var aresp authCodeResponse
	err = json.Unmarshal(data, &aresp)
	if err != nil {
		log.Printf("cannot exchange code for token: unmarshal: %s", err)
		return
	}

	c.Token = aresp.AccessToken
	c.ExpiresAt = time.Now().Add(time.Second * time.Duration(aresp.ExpiresIn))
	log.Printf("YouTube token refreshed, now expires at %s", c.ExpiresAt)
}

func (c *YoutubeClient) Messages(ctx context.Context) (<-chan YoutubeMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ExpiresAt.Before(time.Now()) && !c.ExpiresAt.IsZero() {
		c.Token = ""
		return nil, fmt.Errorf("token has expired at %s", c.ExpiresAt)
	}

	if c.Token == "" {
		return nil, fmt.Errorf("youtube token is not set")
	}

	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(nil)))
	if c.socksProxy != "" {
		dialer, err := newSOCKS5Dialer(c.socksProxy, 10*time.Second)
		if err != nil {
			return nil, fmt.Errorf("youtube grpc proxy dialer error: %w", err)
		}
		opts = append(opts, grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return dialWithProxyContext(ctx, dialer, "tcp", addr)
		}))
	}

	conn, err := grpc.Dial(yt_SERVER_ADDR, opts...)
	if err != nil {
		return nil, fmt.Errorf("youtube grpc dial error: %w", err)
	}
	c.streamConn = conn

	client := streamlist.NewV3DataLiveChatMessageServiceClient(conn)

	req := &streamlist.LiveChatMessageListRequest{
		LiveChatId: &c.LiveChatID,
		Part:       []string{"snippet", "authorDetails"},
	}

	md := metadata.Pairs("Authorization", "Bearer "+c.Token)
	ctx = metadata.NewOutgoingContext(ctx, md)

	stream, err := client.StreamList(ctx, req)
	if err != nil {
		c.streamConn = nil
		conn.Close()
		return nil, fmt.Errorf("youtube grpc could not stream messages: %w", err)
	}

	if c.ExpiresAt.Sub(time.Now()) < time.Minute*time.Duration(30) {
		go c.Refresh()
	}

	ch := make(chan YoutubeMessage)
	go func(chan<- YoutubeMessage) {
		defer func() {
			conn.Close()
			c.mu.Lock()
			if c.streamConn == conn {
				c.streamConn = nil
			}
			c.mu.Unlock()
		}()
		defer close(ch)

		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				stream, err = client.StreamList(ctx, req)
				if err != nil {
					log.Printf("failed to restart message stream: %v", err)
					break
				}
				continue
			}
			if err != nil {
				log.Printf("failed to receive a message: %v", err)
				break
			}

			req.PageToken = msg.NextPageToken

			for _, item := range msg.Items {
				if item.GetAuthorDetails().GetIsChatOwner() {
					continue
				}

				snippet := item.GetSnippet()
				if snippet.GetType() != streamlist.LiveChatMessageSnippet_TypeWrapper_TEXT_MESSAGE_EVENT {
					continue
				}

				author := item.GetAuthorDetails()
				ch <- YoutubeMessage{
					From: author.GetDisplayName(),
					Body: snippet.GetTextMessageDetails().GetMessageText(),
				}
			}
		}
	}(ch)

	return ch, nil
}

// vim: ai:ts=8:sw=8:noet:syntax=go
