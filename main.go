package main

import (
	"crypto/rand"
	"encoding/base64"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	"github.com/GeertJohan/go.rice"
	"github.com/gin-gonic/contrib/renders/multitemplate"
	"github.com/gin-gonic/gin"
)

func InitAssetsTemplates(r *gin.Engine, tbox, abox *rice.Box) error {
	var err error
	var data string
	var tmpl *template.Template

	var names, pnames []string

	tbox.Walk("", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if path != "" {
			if strings.HasSuffix(path, ".swp") {
				return nil
			}

			if strings.HasPrefix(path, "_") {
				pnames = append(pnames, path)
			} else {
				names = append(names, path)
			}
		}
		return nil
	})

	render := multitemplate.New()
	ptmpls := make(map[string]*template.Template)
	for _, pname := range pnames {
		if strings.HasSuffix(pname, ".swp") {
			continue
		}
		if data, err = tbox.String(pname); err != nil {
			return err
		}
		pname = strings.TrimSuffix(pname, ".html")
		if tmpl, err = template.New(pname).Parse(data); err != nil {
			return err
		}
		ptmpls[pname] = tmpl
	}
	for _, name := range names {
		if data, err = tbox.String(name); err != nil {
			return err
		}
		if tmpl, err = template.New(name).Parse(data); err != nil {
			return err
		}
		for pname, ptmpl := range ptmpls {
			tmpl.AddParseTree(pname, ptmpl.Tree)
		}
		render.Add(name, tmpl)
	}
	r.HTMLRender = render

	h := abox.HTTPBox()
	abox.Walk("", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".swp") {
			return nil
		}

		if path != "" {
			r.GET(path, func(c *gin.Context) {
				c.FileFromFS(path, h)
			})
		}
		return nil
	})
	return nil
}

func main() {
	twitch := NewTwitchClient()
	rcon := NewRconClient
	tbox := rice.MustFindBox("templates")
	abox := rice.MustFindBox("assets")

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	if err := InitAssetsTemplates(r, tbox, abox); err != nil {
		log.Fatal(err)
	}

	csrf_buf := make([]byte, 16)
	_, err := rand.Read(csrf_buf)
	if err != nil {
		log.Fatalf("cannot read random bytes: %s", err)
	}
	csrf := base64.RawURLEncoding.EncodeToString(csrf_buf)

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

		if p.State != csrf {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error": "bad_csrf",
			})
			return
		}

		twitch.Token = p.Token
		err := twitch.Prepare(c.Request.Context())

		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"error":       "twitch_error",
				"description": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"CSRF":   csrf,
			"Twitch": twitch,
			"Rcon":   rcon,
		})
	})

	l, err := net.Listen("tcp", "localhost:8666")
	if err != nil {
		log.Panic(err)
	}

	log.Println("Starting up a server on http://localhost:8666/")
	go func() {
		return
		switch runtime.GOOS {
		case "linux":
			exec.Command("xdg-open", "http://localhost:8666/").Start()
		case "windows":
			exec.Command(
				"rundll32",
				"url.dll,FileProtocolHandler",
				"http://localhost:8666/",
			).Start()
		case "darwin":
			exec.Command("open", "http://localhost:8666/").Start()
		}
	}()
	log.Panic(r.RunListener(l))
}

// vim: ai:ts=8:sw=8:noet:syntax=go
