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
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mattn/anko/parser"
	"golang.org/x/net/websocket"
)

type ZDRCT struct {
	tw_broadcaster *TwitchClient
	tw_bot         *TwitchClient
	yt_bot         *YoutubeClient
	rcon           *RconClient
	bot            *Bot
	remote         *Remote
	alerter        *Alerter
	config         *Config

	engine   *gin.Engine
	csrf     string
	execPath string
}

func (z *ZDRCT) Init() error {
	if len(os.Args) > 0 {
		dir, _ := filepath.Split(os.Args[0])
		if dir != "" {
			err := os.Chdir(dir)
			if err != nil {
				return fmt.Errorf("cannot cd into %q: %w", dir, err)
			}
		}
	}

	var execErr error
	z.execPath, execErr = os.Executable()
	if execErr != nil {
		log.Printf("cannot get executable path: %s", execErr)
	}

	z.config = &Config{}
	err := z.config.Init()
	if err != nil {
		return fmt.Errorf("cannot init config system: %w", err)
	}
	err = z.config.Load()
	if err != nil {
		return fmt.Errorf("error loading config file: %w", err)
	}

	z.tw_broadcaster = NewTwitchClient(TwitchClientOpts{
		Scopes:     z.config.BroadcasterScopes,
		Purpose:    "broadcaster",
		SocksProxy: z.config.SocksProxy,
	})
	z.tw_bot = NewTwitchClient(TwitchClientOpts{
		Scopes:     DEFAULT_BOT_SCOPES,
		Purpose:    "bot",
		SocksProxy: z.config.SocksProxy,
	})
	z.yt_bot = NewYoutubeClient(YoutubeClientOpts{
		SocksProxy: z.config.SocksProxy,
	})
	z.tw_bot.Scopes = strings.Split(DEFAULT_BOT_SCOPES, ",")
	z.rcon = NewRconClient()
	z.bot = NewBot(z.tw_broadcaster, z.tw_bot, z.yt_bot, z.config.zdrctConfigDir)
	z.bot.RconClient = z.rcon
	z.remote = NewRemote(z.bot, z.config.SocksProxy)
	z.alerter = NewAlerter()
	z.bot.Alerter = z.alerter

	if err := z.initWeb(); err != nil {
		return err
	}

	if err := z.initTwitch(); err != nil {
		return err
	}

	if err := z.initYoutube(); err != nil {
		return err
	}

	if err := z.initAPI(); err != nil {
		return err
	}

	z.rcon.Addr, _ = net.ResolveUDPAddr("udp", z.config.RconAddress)
	z.rcon.Password = z.config.RconPassword

	s := NewSound()
	err = s.Init()
	if err != nil {
		log.Printf("cannot start sound system: %s", err)
	}
	z.alerter.Sound = s
	z.bot.Sound = s

	return nil
}

func (z *ZDRCT) initWeb() error {
	gin.SetMode(gin.ReleaseMode)
	z.engine = gin.New()
	z.engine.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/check_csrf"},
	}))
	z.engine.Use(gin.Recovery())
	if err := z.config.InitAssetsTemplates(z.engine); err != nil {
		return fmt.Errorf("cannot init templates: %w", err)
	}

	csrf_buf := make([]byte, 16)
	_, err := rand.Read(csrf_buf)
	if err != nil {
		return fmt.Errorf("cannot read random bytes: %w", err)
	}
	z.csrf = base64.RawURLEncoding.EncodeToString(csrf_buf)

	if z.config.StaticCSRF != "" {
		z.csrf = z.config.StaticCSRF
	}

	return nil
}

func (z *ZDRCT) initTwitch() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	z.tw_broadcaster.Token = z.config.BroadcasterToken
	if err := z.tw_broadcaster.Prepare(ctx); err != nil {
		z.tw_broadcaster.Token = ""
		z.config.BroadcasterToken = ""
		log.Printf("invalidating broadcaster token: %s", err)
	} else {
		err := z.tw_broadcaster.LoadRewards(ctx)
		if err != nil {
			log.Printf("error loading rewards: %s", err)
		}
		z.remote.SetBroadcaster(z.tw_broadcaster)
	}
	z.tw_bot.Token = z.config.BotToken
	if err := z.tw_bot.Prepare(ctx); err != nil {
		z.tw_bot.Token = ""
		z.config.BotToken = ""
		log.Printf("invalidating bot token: %s", err)
	}
	if z.tw_bot.Token != "" && z.tw_broadcaster.Token != "" && z.config.Script != "" {
		err := z.bot.LoadScript(*z.config)
		if err != nil {
			log.Printf("error loading script: %s", err)
		} else {
			z.bot.AdminName = z.tw_broadcaster.Login
			z.bot.UserName = z.tw_bot.Login
			z.bot.ChannelName = z.tw_broadcaster.Login

			err = z.bot.Start()
			if err != nil {
				log.Printf("cannot start bot: %s", err)
			}

			event := RemoteEvent{}
			event.Config.Buttons = z.bot.GetButtons()
			z.remote.SetConfig(event)
		}
	}

	return nil
}
func (z *ZDRCT) initYoutube() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	z.yt_bot.Token = z.config.YoutubeToken
	z.yt_bot.RefreshToken = z.config.YoutubeRefreshToken
	if z.yt_bot.Token != "" {
		if err := z.yt_bot.Prepare(ctx); err != nil {
			log.Printf("invalidating youtube token: %s", err)
			z.yt_bot.Token = ""
		}
	}
	if z.yt_bot.Token == "" && z.yt_bot.RefreshToken != "" {
		z.yt_bot.Refresh()
	}
	if z.yt_bot.Token != "" {
		z.config.YoutubeToken = z.yt_bot.Token
		z.config.Save()
	}

	return nil
}

func (z *ZDRCT) loadScript(c *gin.Context) error {
	var p struct {
		CSRF   string `form:"csrf"`
		Script string `form:"script"`
	}

	if err := c.ShouldBind(&p); err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return err
	}

	if p.CSRF != z.csrf {
		c.AbortWithStatusJSON(http.StatusOK, gin.H{
			"error": "bad_csrf",
		})
		return errors.New("bad csrf")
	}

	script := strings.TrimSpace(p.Script)
	if script == "" {
		script = z.config.Script
	}

	newConfig := *z.config
	newConfig.Script = script
	err := z.bot.LoadScript(newConfig)
	if err != nil {
		h := gin.H{
			"error":       "script_error",
			"description": err.Error(),
		}

		if perr, ok := err.(*parser.Error); ok {
			h["line"] = perr.Pos.Line
			h["column"] = perr.Pos.Column
		}

		c.AbortWithStatusJSON(http.StatusOK, h)
		return err
	}

	z.config.Script = script
	if err := z.config.Save(); err != nil {
		log.Printf("cannot save config: %s", err)
	}

	event := RemoteEvent{}
	event.Config.Buttons = z.bot.GetButtons()
	z.remote.SetConfig(event)

	return nil
}

func (z *ZDRCT) initAPI() error {
	z.engine.GET("/oauth", func(c *gin.Context) {
		qs := c.Request.URL.Query()
		code := qs.Get("code")
		scope := qs.Get("scope")
		if code == "" || scope == "" {
			c.HTML(http.StatusOK, "oauth.html", nil)
			return
		}

		if strings.HasPrefix(scope, "https://www.googleapis.com/auth/") {
			err := z.yt_bot.AuthCode(c.Request.Context(), "http://localhost:8666/oauth", code)
			if err != nil {
				c.HTML(http.StatusOK, "error.html", gin.H{"Error": err.Error()})
				return
			}

			if rt := z.yt_bot.RefreshToken; rt != "" {
				z.config.YoutubeRefreshToken = rt
				if err := z.config.Save(); err != nil {
					log.Printf("cannot save config: %s", err)
				}
			}

			c.Redirect(http.StatusFound, "/?tab=youtube")
			return
		}

		c.Redirect(http.StatusFound, "/")
	})

	z.engine.POST("/oauth", func(c *gin.Context) {
		var p struct {
			State string `json:"state"`
			Token string `json:"access_token"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		u, err := url.ParseQuery(p.State)
		if err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		user_csrf_token := u.Get("csrf_token")
		if user_csrf_token != z.csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		var twitch_impl *TwitchClient
		user_purpose := u.Get("purpose")
		switch user_purpose {
		case "bot":
			twitch_impl = z.tw_bot
		case "broadcaster":
			twitch_impl = z.tw_broadcaster
		default:
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "no_purpose",
			})
			return
		}

		twitch_impl.Token = p.Token
		err = twitch_impl.Prepare(c.Request.Context())

		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error":       "twitch_error",
				"description": err.Error(),
			})
			return
		}

		z.config.BotToken = z.tw_bot.Token
		z.config.BroadcasterToken = z.tw_broadcaster.Token

		if err := z.config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		if user_purpose == "broadcaster" {
			if err := z.tw_broadcaster.LoadRewards(c.Request.Context()); err != nil {
				log.Printf("error loading rewards: %s", err)
			}
			z.remote.SetBroadcaster(z.tw_broadcaster)
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	z.engine.POST("/oauth/youtube", func(c *gin.Context) {
		var p struct {
			State string `json:"state"`
			Token string `json:"access_token"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		u, err := url.ParseQuery(p.State)
		if err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		user_csrf_token := u.Get("csrf_token")
		if user_csrf_token != z.csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		z.yt_bot.Token = p.Token
		err = z.yt_bot.Prepare(c.Request.Context())

		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error":       "youtube_error",
				"description": err.Error(),
			})
			return
		}

		z.config.YoutubeToken = z.yt_bot.Token

		if err := z.config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	z.engine.POST("/rewards/:id/delete", func(c *gin.Context) {
		err := z.tw_broadcaster.DeleteReward(
			c.Request.Context(),
			Reward{
				RewardCore: RewardCore{
					ID: c.Param("id"),
				},
			},
		)

		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error":       "cannot_delete_reward",
				"description": err.Error(),
			})
			return
		}

		if c.Query("xhr") == "" {
			c.Redirect(http.StatusFound, "/?tab=twitch")
		} else {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		}
	})

	z.engine.POST("/rewards", func(c *gin.Context) {
		var reward Reward
		if err := c.ShouldBind(&reward); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		log.Printf("CreateReward(%#v)", reward)

		err := z.tw_broadcaster.CreateReward(
			c.Request.Context(),
			&reward,
		)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error":       "cannot_create_reward",
				"description": err.Error(),
			})
			return
		}

		if c.Query("xhr") == "" {
			c.Redirect(http.StatusFound, "/?tab=twitch")
		} else {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		}
	})

	z.engine.POST("/loadscript", func(c *gin.Context) {
		err := z.loadScript(c)
		if err != nil {
			return
		}
		if c.Query("xhr") == "" {
			c.Redirect(http.StatusFound, "/?tab=script")
		} else {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		}
	})

	z.engine.GET("/check_csrf", func(c *gin.Context) {
		if c.Query("csrf") != z.csrf {
			c.JSON(http.StatusOK, gin.H{"valid": false})
		} else {
			c.JSON(http.StatusOK, gin.H{"valid": true})
		}
	})

	z.engine.GET("/youtube/api/:method", func(c *gin.Context) {
		if c.Query("csrf") != z.csrf {
			c.JSON(http.StatusOK, gin.H{"valid": false})
			return
		}

		kv := map[string]string{}
		for k := range c.Request.URL.Query() {
			if k == "csrf" {
				continue
			}
			kv[k] = c.Request.URL.Query().Get(k)
		}
		result, err := z.yt_bot.GetAPI(
			c.Request.Context(),
			c.Param("method"),
			kv,
		)

		if err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"ok":     true,
			"result": result,
		})
	})

	z.engine.POST("/connect", func(c *gin.Context) {
		var p struct {
			CSRF    string `form:"csrf"`
			Token   string `form:"token"`
			Purpose string `form:"purpose"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != z.csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		var twitch_impl *TwitchClient
		switch p.Purpose {
		case "bot":
			twitch_impl = z.tw_bot
		case "broadcaster":
			twitch_impl = z.tw_broadcaster
		default:
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "no_purpose",
			})
			return
		}

		if p.Token != "" {
			twitch_impl.Token = p.Token
			err := twitch_impl.Prepare(c.Request.Context())
			if err != nil {
				twitch_impl.Token = ""
			} else {
				c.Redirect(http.StatusFound, "/?tab=twitch")
				return
			}
		}

		z.config.BroadcasterToken = z.tw_broadcaster.Token
		z.config.BotToken = z.tw_bot.Token
		if err := z.config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		authURL := twitch_impl.GetAuthLink("http://localhost:8666/oauth", p.CSRF)
		var userDataDir string
		if p.Purpose == "bot" && runtime.GOOS == "windows" {
			userDataDir = filepath.Join(z.config.zdrctConfigDir, "webview_bot")
		}
		warning, err := openBrowser(authURL, z.config, z.execPath, userDataDir)
		if err != nil {
			log.Printf("cannot open Twitch OAuth browser: %s", err)
			description := fmt.Sprintf("cannot open Twitch OAuth browser: %s\n\nOpen this URL manually: %s", err, authURL)
			if warning != "" {
				description = fmt.Sprintf("%s\n\nThe fallback browser also failed: %s\n\nOpen this URL manually: %s", warning, err, authURL)
			}
			c.HTML(http.StatusOK, "error.html", gin.H{
				"Error": description,
			})
			return
		}
		if warning != "" {
			log.Printf("Twitch OAuth browser warning: %s", warning)
			c.HTML(http.StatusOK, "error.html", gin.H{
				"Title": "Browser notice",
				"Error": warning,
			})
			return
		}

		c.Redirect(http.StatusFound, "/?tab=twitch")
	})

	z.engine.POST("/youtube/live", func(c *gin.Context) {
		var p struct {
			CSRF       string `form:"csrf"`
			LiveChatID string `form:"live_chat_id"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != z.csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		z.yt_bot.LiveChatID = p.LiveChatID
		c.Redirect(http.StatusFound, "/?tab=youtube")
	})

	z.engine.POST("/youtube/channel", func(c *gin.Context) {
		var p struct {
			CSRF      string `form:"csrf"`
			ChannelID string `form:"channel_id"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != z.csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		z.config.YoutubeChannelID = p.ChannelID
		if err := z.config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		c.Redirect(http.StatusFound, "/?tab=youtube")
	})

	z.engine.POST("/connect/youtube", func(c *gin.Context) {
		var p struct {
			CSRF  string `form:"csrf"`
			Token string `form:"token"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != z.csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		if p.Token != "" {
			z.yt_bot.Token = p.Token
			err := z.yt_bot.Prepare(c.Request.Context())
			if err != nil {
				z.yt_bot.Token = ""
			} else {
				c.Redirect(http.StatusFound, "/?tab=youtube")
				return
			}
		}

		z.config.YoutubeToken = z.yt_bot.Token
		if err := z.config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		authURL := z.yt_bot.GetAuthLink("http://localhost:8666/oauth", p.CSRF)
		warning, err := openBrowser(authURL, z.config, z.execPath, "")
		if err != nil {
			log.Printf("cannot open YouTube OAuth browser: %s", err)
			description := fmt.Sprintf("cannot open YouTube OAuth browser: %s\n\nOpen this URL manually: %s", err, authURL)
			if warning != "" {
				description = fmt.Sprintf("%s\n\nThe fallback browser also failed: %s\n\nOpen this URL manually: %s", warning, err, authURL)
			}
			c.HTML(http.StatusOK, "error.html", gin.H{
				"Error": description,
			})
			return
		}
		if warning != "" {
			log.Printf("YouTube OAuth browser warning: %s", warning)
			c.HTML(http.StatusOK, "error.html", gin.H{
				"Title": "Browser notice",
				"Error": warning,
			})
			return
		}

		c.Redirect(http.StatusFound, "/?tab=youtube")
	})

	z.engine.POST("/disconnect", func(c *gin.Context) {
		var p struct {
			CSRF    string `form:"csrf"`
			Purpose string `form:"purpose"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != z.csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		var twitch_impl *TwitchClient
		switch p.Purpose {
		case "bot":
			z.config.BotToken = ""
			twitch_impl = z.tw_bot
		case "broadcaster":
			z.config.BroadcasterToken = ""
			twitch_impl = z.tw_broadcaster
		default:
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "no_purpose",
			})
			return
		}

		twitch_impl.UserID = 0
		twitch_impl.Login = ""
		twitch_impl.Token = ""

		if err := z.config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		c.Redirect(http.StatusFound, "/?tab=twitch")
	})

	z.engine.POST("/open/twitch", func(c *gin.Context) {
		var p struct {
			CSRF string `form:"csrf"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != z.csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		_, err := openBrowser("https://twitch.tv/", z.config, z.execPath, "")
		if err != nil {
			log.Printf("cannot open Twitch browser: %s", err)
		}

		c.Redirect(http.StatusFound, "/?tab=twitch")
	})

	z.engine.POST("/disconnect/youtube", func(c *gin.Context) {
		var p struct {
			CSRF string `form:"csrf"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != z.csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		z.config.YoutubeToken = ""
		z.yt_bot.Login = ""
		z.yt_bot.Token = ""

		if err := z.config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		c.Redirect(http.StatusFound, "/?tab=youtube")
	})

	z.engine.GET("/alerts", func(c *gin.Context) {
		c.HTML(http.StatusOK, "alerts.html", nil)
	})

	z.engine.GET("/random-video", func(c *gin.Context) {
		mediaFiles, err := z.config.MediaFileURLs()
		if err != nil {
			log.Printf("cannot list media files: %s", err)
			mediaFiles = []string{}
		}
		c.HTML(http.StatusOK, "obs-random-videos.html", gin.H{"MediaFiles": mediaFiles})
	})

	if z.config.MediaFilesDir != "" {
		z.engine.Static("/mediafiles", z.config.MediaFilesDir)
	}

	z.engine.GET("/alerts/ws", func(c *gin.Context) {
		handler := websocket.Handler(func(ws *websocket.Conn) {
			defer ws.Close()
			enc := json.NewEncoder(ws)
			ch := z.alerter.Subscribe()
			for {
				select {
				case <-c.Request.Context().Done():
					return
				case alert := <-ch:
					err := enc.Encode(alert)
					if err != nil {
						log.Printf("cannot send alert: %s", err)
						return
					}
				}
			}
		})
		handler.ServeHTTP(c.Writer, c.Request)
	})

	z.engine.POST("/startbot", func(c *gin.Context) {
		err := z.loadScript(c)
		if err != nil {
			return
		}

		err = z.bot.Start()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error":       "bot_error",
				"description": err.Error(),
			})
			return
		}

		if c.Query("xhr") == "" {
			c.Redirect(http.StatusFound, "/?tab=script")
		} else {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		}
	})

	z.engine.POST("/rundoom", func(c *gin.Context) {
		var p struct {
			Path string `form:"path"`
			Args string `form:"args"`
			CSRF string `form:"csrf"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != z.csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		var args []string
		for _, line := range strings.Split(p.Args, "\n") {
			line = strings.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			if line[0] == '#' {
				continue
			}

			if line[0] == '-' || line[0] == '+' {
				idx := strings.IndexRune(line, ' ')
				if idx == -1 {
					args = append(args, line)
				} else {
					args = append(args, line[0:idx], line[idx+1:])
				}
			} else {
				args = append(args, line)
			}
		}

		err := inject(p.Path, args...)
		if err != nil {
			c.HTML(http.StatusOK, "error.html", gin.H{"Error": err.Error()})
			return
		}

		z.config.DoomExe = p.Path
		z.config.DoomArgs = p.Args
		if err := z.config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		c.Redirect(http.StatusFound, "/?tab=doomexe")
	})

	z.engine.POST("/rcon/config", func(c *gin.Context) {
		var p struct {
			Addr     string `form:"addr"`
			Password string `form:"password"`
			CSRF     string `form:"csrf"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != z.csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		z.rcon.Close()

		err := z.rcon.Connect(p.Addr, p.Password)
		if err != nil {
			c.HTML(http.StatusOK, "error.html", gin.H{"Error": err.Error()})
			return
		}

		z.config.RconAddress = p.Addr
		z.config.RconPassword = p.Password
		if err := z.config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		c.Redirect(http.StatusFound, "/?tab=rcon")
	})

	z.engine.POST("/rcon", func(c *gin.Context) {
		var p struct {
			Command string `form:"command"`
			CSRF    string `form:"csrf"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != z.csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		if !z.rcon.IsOnline() {
			c.HTML(http.StatusOK, "error.html", gin.H{"Error": "not connected"})
			return
		}

		err := z.rcon.Command(p.Command)
		if err != nil {
			c.HTML(http.StatusOK, "error.html", gin.H{"Error": err.Error()})
			return
		}

		c.Redirect(http.StatusFound, "/?tab=rcon")
	})

	z.engine.POST("/settings", func(c *gin.Context) {
		var p struct {
			TtsEndpoint            string `form:"tts_endpoint"`
			RconAutoStart          bool   `form:"rcon_auto_start"`
			YoutubeAutoStart       bool   `form:"youtube_auto_start"`
			NoMappedRewardCommands bool   `form:"no_mapped_reward_commands"`
			SoundVolume            int    `form:"sound_volume"`
			StaticCSRF             string `form:"static_csrf"`
			SocksProxy             string `form:"socks_proxy"`
			AssetMounts            string `form:"asset_mounts"`
			MediaFilesDir          string `form:"media_files_dir"`

			CSRF string `form:"csrf"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != z.csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		z.config.TtsEndpoint = p.TtsEndpoint
		z.config.RconAutoStart = p.RconAutoStart
		z.config.YoutubeAutoStart = p.YoutubeAutoStart
		z.config.NoMappedRewardCommands = p.NoMappedRewardCommands
		z.config.SoundVolume = p.SoundVolume
		z.config.StaticCSRF = p.StaticCSRF
		var assetMounts []string
		for _, line := range strings.Split(p.AssetMounts, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				assetMounts = append(assetMounts, line)
			}
		}
		z.config.AssetMounts = assetMounts
		z.config.MediaFilesDir = strings.TrimSpace(p.MediaFilesDir)
		oldSocksProxy := z.config.SocksProxy
		z.config.SocksProxy = strings.TrimSpace(p.SocksProxy)
		z.tw_broadcaster.SetSocksProxy(z.config.SocksProxy)
		z.tw_bot.SetSocksProxy(z.config.SocksProxy)
		z.yt_bot.SetSocksProxy(z.config.SocksProxy)
		z.remote.SetSocksProxy(z.config.SocksProxy)
		if oldSocksProxy != z.config.SocksProxy {
			z.bot.ReconnectIRC()
			z.yt_bot.ReconnectLiveChat()
		}

		if err := z.config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		c.Redirect(http.StatusFound, "/?tab=settings")
	})

	z.engine.GET("/", func(c *gin.Context) {
		tab := c.Query("tab")
		if tab == "" {
			tab = "twitch"
		}
		tables, err := z.config.ReadDir("tables", false)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				log.Printf("cannot read tables dir: %s", err)
			}
		} else {
			for i, table := range tables {
				tables[i] = strings.TrimSuffix(
					filepath.Base(table),
					".json",
				)
			}
		}

		c.HTML(http.StatusOK, "index.html", gin.H{
			"CSRF":      z.csrf,
			"Twitch":    z.tw_broadcaster,
			"TwitchBot": z.tw_bot,
			"Youtube":   z.yt_bot,
			"Rcon":      z.rcon,
			"Bot":       z.bot,
			"Tab":       tab,
			"Config":    z.config,
			"Tables":    tables,
			"Version":   Version,
			"BuildTime": BuildTime,
		})
	})

	z.engine.POST("/upload/assets/:name", func(c *gin.Context) {
		data, err := c.GetRawData()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]string{
				"error":       "get_asset_failed",
				"description": err.Error(),
			})
			log.Printf("get asset failed: %s", err)
			return
		}

		err = z.config.WriteAsset(c.Param("name"), data)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]string{
				"error":       "write_asset_failed",
				"description": err.Error(),
			})
			log.Printf("write asset failed: %s", err)
			return
		}

		c.JSON(http.StatusOK, map[string]bool{"ok": true})
	})

	z.engine.GET("/tables", func(c *gin.Context) {
		var p struct {
			CSRF  string `form:"csrf"`
			Name  string `form:"name"`
			Table *Table
		}
		var err error

		if err := c.ShouldBindQuery(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != z.csrf {
			c.JSON(http.StatusBadRequest, gin.H{"valid": false})
			return
		}

		p.Table, err = ReadTable(z.config.zdrctConfigDir, p.Name)
		if err != nil {
			c.HTML(http.StatusInternalServerError, "error.html", gin.H{
				"Error": err.Error(),
			})
			return
		}
		c.HTML(http.StatusOK, "table-editor.html", p)
	})

	z.engine.POST("/tables", func(c *gin.Context) {
		var p struct {
			CSRF  string `json:"csrf"`
			Name  string `json:"name"`
			Table *Table `json:"table"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != z.csrf && p.Table == nil {
			c.JSON(http.StatusBadRequest, gin.H{"valid": false})
			return
		}

		p.Table.Name = p.Name
		p.Table.filepath = filepath.Join(
			z.config.zdrctConfigDir,
			"tables",
			url.QueryEscape(p.Name)+".json",
		)
		if err := p.Table.Save(); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.HTML(http.StatusOK, "table-editor.html", p)
	})
	return nil
}

func (z *ZDRCT) rconAutoStarter() {
	time.Sleep(2 * time.Second)

	t := time.NewTicker(time.Second)
	defer t.Stop()
	for range t.C {
		if z.rcon.IsOnline() {
			continue
		}

		log.Println("Auto-connecting to rcon server...")
		err := z.rcon.Connect(z.config.RconAddress, z.config.RconPassword)
		if err != nil {
			log.Printf("Error: %s", err)
		} else {
			log.Println("Auto-connect succeeded.")
			time.Sleep(time.Second * 5)
		}
	}
}

func (z *ZDRCT) youtubeAutoResolver() {
	delay := time.Duration(0)
	for {
		time.Sleep(delay)
		if delay == 0 {
			delay = time.Second * 30
		} else {
			delay = delay * 3
			if delay > time.Minute*15 {
				delay = time.Minute * 15
			}
		}
		if z.config.YoutubeChannelID == "" || z.yt_bot.Token == "" || z.yt_bot.LiveChatID != "" {
			continue
		}

		log.Println("Resolving YouTube live chat ID...")
		lcid, err := z.yt_bot.ResolveChannel(z.config.YoutubeChannelID)
		if err != nil {
			log.Printf("Error: %s", err)
		} else {
			z.yt_bot.LiveChatID = lcid
			log.Printf("Resolved YouTube live chat: %q", lcid)
			delay = time.Minute
		}
	}
}

func (z *ZDRCT) Start() error {
	if z.config.RconAutoStart {
		go z.rconAutoStarter()
	}

	if z.config.YoutubeAutoStart {
		go z.youtubeAutoResolver()
	}

	l, err := net.Listen("tcp", "localhost:8666")
	if err != nil {
		return err
	}

	log.Println("Starting up a server on http://localhost:8666/")
	go func() {
		url := "http://localhost:8666/"
		switch runtime.GOOS {
		case "linux":
			go exec.Command("xdg-open", url).Run()
		case "windows":
			go exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Run()
		case "darwin":
			go exec.Command("open", url).Run()
		}
	}()

	return z.engine.RunListener(l)
}

func main() {
	z := &ZDRCT{}
	if err := z.Init(); err != nil {
		log.Fatal(err)
	}

	log.Panic(z.Start())
}

// vim: ai:ts=8:sw=8:noet:syntax=go
