package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
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

func (c *Config) Save() error {
	for _, fn := range []func() error{c.SaveConfig, c.SaveScript} {
		err := fn()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) SaveConfig() error {
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

func (c *Config) SaveScript() error {
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

// vim: ai:ts=8:sw=8:noet:syntax=go
