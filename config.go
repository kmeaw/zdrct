package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/contrib/renders/multitemplate"
	"github.com/gin-gonic/gin"
)

type Config struct {
	BroadcasterToken string `json:"broadcaster_token,omitempty"`
	BotToken         string `json:"bot_token,omitempty"`
	Script           string `json:"-"`
	DoomExe          string `json:"doom_exe"`
	DoomArgs         string `json:"doom_args"`
	RconAddress      string `json:"rcon_address,omitempty"`
	RconPassword     string `json:"rcon_password,omitempty"`

	zdrctConfigDir string
}

func (c *Config) SetDefaultScript() {
	c.Script = `
cmd_echo = func(flds...) {
  reply("echo for %q: %q", from, join(flds, " "))
}

cmd_rcon = func(flds...) {
  if from != admin {
    reply("Forbidden, %q != %q.", from, admin)
  } else {
    rcon(join(flds, " "))
  }
}

cmd_balance = func() {
  reply("You have %d credits.", balance(from))
}

func redeem(price, cmd) {
  return func(args...) {
    b = balance(from)
    if b < price {
      reply("You have %d credits, but this command requires %d.", balance(from), price)
    }
    result = cmd(args...)
    if result {
      set_balance(from, b - price)
    }
  }
}

cmd_gargoyle = redeem(10, func() {
  if rcon("summon HereticImp") {
    alert(sprintf("%s has summoned a gargoyle!", from), "gargoyle.png", "impsit.mp3")
    reply("%s has summoned a gargoyle!", from)
  }
})

cmd_flask = redeem(5, func() {
  if rcon("summon " + roll(3.0, "ArtiHealth", 1.0, "ActivatedTimeBomb")) {
    reply("%s, thank you!", from)
    alert(sprintf("%s has spawned a flask", from), "QuartzFlask.gif", "artiup.mp3")
    return true
  }
})
`
}

func (c *Config) SetDefaults() {
	c.BroadcasterToken = ""
	c.BotToken = ""

	c.RconAddress = "127.0.0.1:10666"
	c.RconPassword = ""

	c.DoomExe = "path/to/zdoom.exe"
	c.DoomArgs = "+sv_cheats 1\n-skill 5\n-warp 1 2\n"
}

func (c *Config) Init() error {
	cfgdir, err := os.UserConfigDir()
	if err != nil {
		return err
	}

	c.zdrctConfigDir = filepath.Join(cfgdir, "zdrct")

	err = os.MkdirAll(c.zdrctConfigDir, 0777)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}

	return nil
}

func (c *Config) Load() error {
	for _, fn := range []func() error{c.LoadConfig, c.LoadScript} {
		err := fn()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) LoadConfig() error {
	f, err := os.Open(filepath.Join(c.zdrctConfigDir, "config.json"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			c.SetDefaults()
			return nil
		}

		return err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	err = dec.Decode(c)
	if err != nil {
		return err
	}

	return nil
}

func (c *Config) LoadScript() error {
	b, err := os.ReadFile(filepath.Join(c.zdrctConfigDir, "script.anko"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			c.SetDefaultScript()
			return nil
		}

		return nil
	}

	c.Script = string(b)
	return nil
}

func (c Config) Save() error {
	for _, fn := range []func() error{c.SaveConfig, c.SaveScript} {
		err := fn()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c Config) SaveConfig() error {
	f, err := os.OpenFile(filepath.Join(c.zdrctConfigDir, "config.json"), os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("    ", "")
	err = enc.Encode(c)
	if err != nil {
		return err
	}

	return nil
}

func (c Config) SaveScript() error {
	f, err := os.OpenFile(filepath.Join(c.zdrctConfigDir, "script.anko"), os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write([]byte(c.Script))
	if err != nil {
		return err
	}

	return nil
}

func (c Config) ReadDir(dirname string) ([]string, error) {
	locals := make(map[string]bool)
	entries, err := os.ReadDir(filepath.Join(c.zdrctConfigDir, dirname))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}

	result := make([]string, 0, len(entries))

	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}

		if strings.HasSuffix(entry.Name(), ".swp") || strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(entry.Name(), "~") {
			continue
		}

		locals[entry.Name()] = true
		result = append(result, filepath.Join(c.zdrctConfigDir, dirname, entry.Name()))
	}

	entries, err = os.ReadDir(dirname)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}

		if strings.HasSuffix(entry.Name(), ".swp") || strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(entry.Name(), "~") {
			continue
		}

		if locals[entry.Name()] {
			continue
		}

		result = append(result, filepath.Join(dirname, entry.Name()))
	}

	return result, nil
}

func (c Config) InitAssetsTemplates(r *gin.Engine) error {
	var err error
	var data []byte
	var tmpl *template.Template

	var names, pnames []string

	template_files, err := c.ReadDir("templates")
	if err != nil {
		return err
	}
	for _, name := range template_files {
		if strings.HasPrefix(filepath.Base(name), "_") {
			pnames = append(pnames, name)
		} else {
			names = append(names, name)
		}
	}

	funcs := template.FuncMap{
		"join": strings.Join,
	}

	render := multitemplate.New()
	ptmpls := make(map[string]*template.Template)
	for _, pname := range pnames {
		if data, err = os.ReadFile(pname); err != nil {
			return fmt.Errorf("cannot open %q from tbox while iterating over pnames: %w", pname, err)
		}
		pname = strings.TrimSuffix(filepath.Base(pname), ".html")
		if tmpl, err = template.New(pname).Funcs(funcs).Parse(string(data)); err != nil {
			return fmt.Errorf("cannot parse template %q: %w", pname, err)
		}
		ptmpls[pname] = tmpl
	}
	for _, name := range names {
		if data, err = os.ReadFile(name); err != nil {
			return fmt.Errorf("cannot open %q from tbox while iterating over names: %w", name, err)
		}
		if tmpl, err = template.New(filepath.Base(name)).Funcs(funcs).Parse(string(data)); err != nil {
			return fmt.Errorf("cannot parse template %q: %w", name, err)
		}
		for pname, ptmpl := range ptmpls {
			tmpl.AddParseTree(pname, ptmpl.Tree)
		}
		render.Add(filepath.Base(name), tmpl)
	}
	r.HTMLRender = render

	asset_files, err := c.ReadDir("assets")
	if err != nil {
		return err
	}
	for _, name := range asset_files {
		name := name
		r.GET(filepath.Base(name), func(c *gin.Context) {
			c.File(name)
		})
	}
	return nil
}

// vim: ai:ts=8:sw=8:noet:syntax=go
