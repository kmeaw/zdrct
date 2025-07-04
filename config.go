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

var Assets = map[string]string{}

type Config struct {
	BroadcasterToken string `json:"broadcaster_token,omitempty"`
	BotToken         string `json:"bot_token,omitempty"`
	Script           string `json:"-"`
	DoomExe          string `json:"doom_exe"`
	DoomArgs         string `json:"doom_args"`
	RconAddress      string `json:"rcon_address,omitempty"`
	RconPassword     string `json:"rcon_password,omitempty"`

	TtsEndpoint            string `json:"tts_endpoint,omitempty"`
	RconAutoStart          bool   `json:"rcon_auto_start"`
	NoMappedRewardCommands bool   `json:"no_mapped_reward_commands"`
	SoundVolume            int    `json:"sound_volume,omitempty"`

	zdrctConfigDir string
}

func (c *Config) SetDefaultScript() {
	c.Script = `
cmd_event_join = func() {
  debug("%q has joined the channel.", from())
}

cmd_event_part = func() {
  debug("%q has left the channel.", from())
}

cmd_echo = func(flds...) {
  reply("echo for %q: %q", from(), join(flds, " "))
}

cmd_help = func() {
  reply("Available commands: %v", list_cmds())
}

cmd_rcon = func(flds...) {
  if from() != admin {
    reply("Forbidden, %q != %q.", from(), admin)
  } else {
    rcon(join(flds, " "))
  }
}

cmd_balance = func() {
  reply("You have %d credits.", balance(from()))
}

func redeem(price, cmd) {
  return func(args...) {
    if is_reward() {
      return cmd(args...)
    }
    b = balance(from())
    if b < price {
      reply("You have %d credits, but this command requires %d.", balance(from()), price)
      return false
    }
    result = cmd(args...)
    if result {
      set_balance(from(), b - price)
    }
  }
}

func spawn_actor(a) {
  if rcon("summon %s", a.ID) {
    actor_alert(a, from())
    actor_reply(a, from())
    sleep(3)
    rcon("summon CrossbowAmmo")
    return true
  } else {
    return false
  }
}

Golem = new(Actor)
Golem.ID = "Mummy"
Golem.Name = "Golem"
Golem.AlertText = "{{ .From }} has summoned a {{ .Actor.Name }}!"
Golem.AlertImage = "golem.png"
Golem.AlertSound = "mumsit.mp3"
Golem.Reply = "{{ .From }} has summoned a {{ .Actor.Name }}!"

Gargoyle = new(Actor)
Gargoyle.ID = "HereticImp"
Gargoyle.Name = "Gargoyle"
Gargoyle.AlertText = "{{ .From }} has summoned a flying {{ .Actor.Name }}!"
Gargoyle.AlertImage = "gargoyle.png"
Gargoyle.AlertSound = "impsit.mp3"
Gargoyle.Reply = "{{ .From }} has summoned a flying {{ .Actor.Name }}!"

cmd_golem = redeem(10, func() {
  return spawn_actor(Golem)
})

cmd_gargoyle = redeem(10, func() {
  return spawn_actor(Gargoyle)
})

cmd_random = redeem(40, func() {
  return spawn_actor(roll(1.0, Golem, 1.0, Gargoyle))
})

cmd_flask = redeem(5, func() {
  if rcon("summon " + roll(3.0, "ArtiHealth", 1.0, "ActivatedTimeBomb")) {
    reply("%s, thank you!", from())
    alert(sprintf("%s has spawned a flask", from()), "QuartzFlask.gif", "artiup.mp3")
    return true
  }
})

cmd_tome = redeem(20, func() {
  if rcon("summon ArtiTomeOfPower") {
    reply("%s, thank you for this powerful artifact!", from())
    alert(sprintf("%s has summoned a tome of power", from()), "ArtiTome.png", "artiup.mp3")
    return true
  }
})

cmd_bomb = redeem(20, func() {
  alert(sprintf("%s has summoned a time bomb!", from()), "bomb.png", "artiup.mp3")
  sleep(3)
  reply("%s, do you really want me to fail?", from())
  return rcon("summon ActivatedTimeBomb")
})

button_tome = new(Command)
button_tome.Cmd = "tome"
button_tome.Text = "Tome of Power"
button_tome.Image = "ArtiTome.png"
add_command(button_tome)

button_random = new(Command)
button_random.Cmd = "random"
button_random.Text = "Random"
button_random.Image = "random.png"
add_command(button_random)

button_flask = new(Command)
button_flask.Cmd = "flask"
button_flask.Text = "Quartz Flask"
button_flask.Image = "QuartzFlask.gif"
add_command(button_flask)

button_gargoyle = new(Command)
button_gargoyle.Cmd = "gargoyle"
button_gargoyle.Text = "Gargoyle"
button_gargoyle.Image = "gargoyle.png"
add_command(button_gargoyle)

button_golem = new(Command)
button_golem.Cmd = "golem"
button_golem.Text = "Golem"
button_golem.Image = "golem.png"
add_command(button_golem)

reward_flask = new(Reward)
reward_flask.Cost = 2
reward_flask.Title = "Flask"
reward_flask.IsEnabled = true
map_reward(reward_flask, button_flask)

reward_random = new(Reward)
reward_random.Cost = 5
reward_random.Title = "Random"
reward_random.IsEnabled = true
map_reward(reward_random, button_random)
`
}

func (c *Config) SetDefaults() {
	c.BroadcasterToken = ""
	c.BotToken = ""

	c.RconAddress = "127.0.0.1:10666"
	c.RconPassword = ""

	c.DoomExe = "path/to/zdoom.exe"
	c.DoomArgs = "+sv_cheats 1\n-skill 5\n-warp 1 2\n"

	c.TtsEndpoint = ""
	c.RconAutoStart = false
	c.NoMappedRewardCommands = false
	c.SoundVolume = 100
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

	c.SoundVolume = 100
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
	f, err := os.OpenFile(filepath.Join(c.zdrctConfigDir, "config.json"), os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
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
	f, err := os.OpenFile(filepath.Join(c.zdrctConfigDir, "script.anko"), os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
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

func (c Config) Asset(filename string) string {
	_, err := os.Stat(filepath.Join(c.zdrctConfigDir, "assets", filename))
	if err == nil { // if NO error
		return filepath.Join(c.zdrctConfigDir, "assets", filename)
	}

	_, err = os.Stat(filepath.Join("assets", filename))
	if err == nil { // if NO error
		return filepath.Join("assets", filename)
	}

	return filename
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

	wd, err := os.Getwd()
	if err != nil {
		return nil, err
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

		result = append(result, filepath.Join(wd, dirname, entry.Name()))
	}

	return result, nil
}

func (c Config) WriteAsset(basename string, data []byte) error {
	err := os.MkdirAll(filepath.Join(c.zdrctConfigDir, "assets"), 0777)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}

	name := filepath.Join(c.zdrctConfigDir, "assets", basename)
	return os.WriteFile(name, data, 0777)
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
		Assets[filepath.Base(name)] = name
		r.GET(filepath.Base(name), func(c *gin.Context) {
			c.File(name)
		})
	}
	return nil
}

// vim: ai:ts=8:sw=8:noet:syntax=go
