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

func main() {
	if len(os.Args) > 0 {
		dir, _ := filepath.Split(os.Args[0])
		if dir != "" {
			err := os.Chdir(dir)
			if err != nil {
				log.Fatalf("cannot cd into %q: %s", dir, err)
			}
		}
	}

	broadcaster := NewTwitchClient(TwitchClientOpts{
		Scopes:  DEFAULT_APP_SCOPES,
		Purpose: "broadcaster",
	})
	bot := NewTwitchClient(TwitchClientOpts{
		Scopes:  DEFAULT_APP_SCOPES,
		Purpose: "bot",
	})
	bot.Scopes = strings.Split(DEFAULT_BOT_SCOPES, ",")
	rcon := NewRconClient()
	ircbot := NewIRCBot(broadcaster, bot)
	ircbot.RconClient = rcon
	remote := NewRemote(ircbot)
	alerter := NewAlerter()
	ircbot.Alerter = alerter

	config := &Config{}
	err := config.Init()
	if err != nil {
		log.Fatalf("cannot init config system: %s", err)
	}
	err = config.Load()
	if err != nil {
		log.Fatalf("error loading config file: %s", err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/check_csrf"},
	}))
	r.Use(gin.Recovery())
	if err := config.InitAssetsTemplates(r); err != nil {
		log.Fatalf("cannot init templates: %s", err)
	}

	csrf_buf := make([]byte, 16)
	_, err = rand.Read(csrf_buf)
	if err != nil {
		log.Fatalf("cannot read random bytes: %s", err)
	}
	csrf := base64.RawURLEncoding.EncodeToString(csrf_buf)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	broadcaster.Token = config.BroadcasterToken
	if err := broadcaster.Prepare(ctx); err != nil {
		broadcaster.Token = ""
		config.BroadcasterToken = ""
		log.Printf("invalidating broadcaster token: %s", err)
	} else {
		err := broadcaster.LoadRewards(ctx)
		if err != nil {
			log.Printf("error loading rewards: %s", err)
		}
		remote.SetBroadcaster(broadcaster)
	}
	bot.Token = config.BotToken
	if err := bot.Prepare(ctx); err != nil {
		bot.Token = ""
		config.BotToken = ""
		log.Printf("invalidating bot token: %s", err)
	}
	if bot.Token != "" && broadcaster.Token != "" && config.Script != "" {
		err = ircbot.LoadScript(config.Script)
		if err != nil {
			log.Printf("error loading script: %s", err)
		} else {
			ircbot.AdminName = broadcaster.Login
			ircbot.UserName = bot.Login
			ircbot.ChannelName = broadcaster.Login

			err = ircbot.Start()
			if err != nil {
				log.Printf("cannot start bot: %s", err)
			}
		}
	}
	cancel()

	rcon.Addr, _ = net.ResolveUDPAddr("udp", config.RconAddress)
	rcon.Password = config.RconPassword

	err = InitSound()
	if err != nil {
		log.Fatalf("cannot start sound system: %s", err)
	}

	r.GET("/oauth", func(c *gin.Context) {
		c.HTML(http.StatusOK, "oauth.html", nil)
	})

	r.POST("/oauth", func(c *gin.Context) {
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
		if user_csrf_token != csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		var twitch_impl *TwitchClient
		user_purpose := u.Get("purpose")
		switch user_purpose {
		case "bot":
			twitch_impl = bot
		case "broadcaster":
			twitch_impl = broadcaster
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

		config.BotToken = bot.Token
		config.BroadcasterToken = broadcaster.Token

		if err := config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		if user_purpose == "broadcaster" {
			if err := broadcaster.LoadRewards(c.Request.Context()); err != nil {
				log.Printf("error loading rewards: %s", err)
			}
			remote.SetBroadcaster(broadcaster)
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.POST("/rewards/:id/delete", func(c *gin.Context) {
		err := broadcaster.DeleteReward(
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

	r.POST("/rewards", func(c *gin.Context) {
		var reward Reward
		if err := c.ShouldBind(&reward); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		log.Printf("CreateReward(%#v)", reward)

		err := broadcaster.CreateReward(
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

	loadScript := func(c *gin.Context) error {
		var p struct {
			Script string `form:"script"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return err
		}

		script := strings.TrimSpace(p.Script)
		if script == "" {
			script = config.Script
		}

		err = ircbot.LoadScript(script)
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

		config.Script = script
		if err := config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		event := RemoteEvent{}
		event.Config.Buttons = ircbot.GetButtons()
		remote.SetConfig(event)

		return nil
	}

	r.POST("/loadscript", func(c *gin.Context) {
		err := loadScript(c)
		if err != nil {
			return
		}
		if c.Query("xhr") == "" {
			c.Redirect(http.StatusFound, "/?tab=script")
		} else {
			c.JSON(http.StatusOK, gin.H{"ok": true})
		}
	})

	r.GET("/check_csrf", func(c *gin.Context) {
		if c.Query("csrf") != csrf {
			c.JSON(http.StatusOK, gin.H{"valid": false})
		} else {
			c.JSON(http.StatusOK, gin.H{"valid": true})
		}
	})

	r.POST("/connect", func(c *gin.Context) {
		var p struct {
			CSRF    string `form:"csrf"`
			Token   string `form:"token"`
			Purpose string `form:"purpose"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if p.CSRF != csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		var twitch_impl *TwitchClient
		switch p.Purpose {
		case "bot":
			twitch_impl = bot
		case "broadcaster":
			twitch_impl = broadcaster
		default:
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "no_purpose",
			})
			return
		}

		if p.Token != "" {
			twitch_impl.Token = p.Token
			err = twitch_impl.Prepare(c.Request.Context())
			if err != nil {
				twitch_impl.Token = ""
			} else {
				c.Redirect(http.StatusFound, "/?tab=twitch")
				return
			}
		}

		config.BroadcasterToken = broadcaster.Token
		config.BotToken = bot.Token
		if err := config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		c.Redirect(http.StatusFound, twitch_impl.GetAuthLink("http://localhost:8666/oauth", p.CSRF))
	})

	r.GET("/alerts", func(c *gin.Context) {
		c.HTML(http.StatusOK, "alerts.html", nil)
	})

	r.GET("/alerts/ws", func(c *gin.Context) {
		handler := websocket.Handler(func(ws *websocket.Conn) {
			defer ws.Close()
			enc := json.NewEncoder(ws)
			ch := alerter.Subscribe()
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

	r.POST("/startbot", func(c *gin.Context) {
		err := loadScript(c)
		if err != nil {
			return
		}

		if bot.BroadcasterID == 0 {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "no_bot_token",
			})
			return
		}

		if broadcaster.BroadcasterID == 0 {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "no_broadcaster_token",
			})
			return
		}

		ircbot.AdminName = broadcaster.Login
		ircbot.UserName = bot.Login
		ircbot.ChannelName = broadcaster.Login

		err = ircbot.Start()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error":       "irc_error",
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

	r.POST("/rundoom", func(c *gin.Context) {
		var p struct {
			Path string `form:"path"`
			Args string `form:"args"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
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

		config.DoomExe = p.Path
		config.DoomArgs = p.Args
		if err := config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		c.Redirect(http.StatusFound, "/?tab=doomexe")
	})

	r.POST("/rcon/config", func(c *gin.Context) {
		var p struct {
			Addr     string `form:"addr"`
			Password string `form:"password"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		rcon.Close()

		err = rcon.Connect(p.Addr, p.Password)
		if err != nil {
			c.HTML(http.StatusOK, "error.html", gin.H{"Error": err.Error()})
			return
		}

		config.RconAddress = p.Addr
		config.RconPassword = p.Password
		if err := config.Save(); err != nil {
			log.Printf("cannot save config: %s", err)
		}

		c.Redirect(http.StatusFound, "/?tab=rcon")
	})

	r.POST("/rcon", func(c *gin.Context) {
		var p struct {
			Command string `form:"command"`
		}

		if err := c.ShouldBind(&p); err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		if !rcon.IsOnline() {
			c.HTML(http.StatusOK, "error.html", gin.H{"Error": "not connected"})
			return
		}

		err = rcon.Command(p.Command)
		if err != nil {
			c.HTML(http.StatusOK, "error.html", gin.H{"Error": err.Error()})
			return
		}

		c.Redirect(http.StatusFound, "/?tab=rcon")
	})

	r.GET("/", func(c *gin.Context) {
		tab := c.Query("tab")
		if tab == "" {
			tab = "twitch"
		}
		c.HTML(http.StatusOK, "index.html", gin.H{
			"CSRF":      csrf,
			"Twitch":    broadcaster,
			"TwitchBot": bot,
			"Rcon":      rcon,
			"IRCBot":    ircbot,
			"Tab":       tab,
			"Config":    config,
		})
	})

	r.POST("/upload/assets/:name", func(c *gin.Context) {
		data, err := c.GetRawData()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, map[string]string{
				"error":       "get_asset_failed",
				"description": err.Error(),
			})
			log.Printf("get asset failed: %s", err)
			return
		}

		err = config.WriteAsset(c.Param("name"), data)
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

	l, err := net.Listen("tcp", "localhost:8666")
	if err != nil {
		log.Panic(err)
	}

	log.Println("Starting up a server on http://localhost:8666/")
	go func() {
		switch runtime.GOOS {
		case "linux":
			go exec.Command("xdg-open", "http://localhost:8666/").Run()
		case "windows":
			go exec.Command(
				"rundll32",
				"url.dll,FileProtocolHandler",
				"http://localhost:8666/",
			).Run()
		case "darwin":
			go exec.Command("open", "http://localhost:8666/").Run()
		}
	}()
	log.Panic(r.RunListener(l))
}

// vim: ai:ts=8:sw=8:noet:syntax=go
