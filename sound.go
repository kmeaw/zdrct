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
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

const antiPop = 4800

type Sound struct {
	sounds map[string]*oto.Player
	cfg    *Config
	opts   *oto.NewContextOptions
	ctx    *oto.Context

	ffmpeg string
	sync.Mutex
}

func NewSound() *Sound {
	cfg := &Config{}
	cfg.Init()

	return &Sound{
		sounds: map[string]*oto.Player{},
		cfg:    cfg,
		opts: &oto.NewContextOptions{
			SampleRate:   48000,
			ChannelCount: 2,
			Format:       oto.FormatSignedInt16LE,
		},
		ctx: nil,
	}
}

func (s *Sound) Init() error {
	otoCtx, readyChan, err := oto.NewContext(s.opts)
	if err != nil {
		return err
	}

	<-readyChan
	s.ctx = otoCtx
	s.ffmpeg = "ffmpeg"
	if e := os.Getenv("ffmpeg"); e != "" {
		s.ffmpeg = e
	} else if runtime.GOOS == "windows" {
		s.ffmpeg = ".\\ffmpeg.exe"
	}
	return nil
}

func (s *Sound) decodeWithFFmpeg(filename string) ([]byte, error) {
	buf := new(bytes.Buffer)
	cmd := exec.Command(
		s.ffmpeg,
		"-i", filename,
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-ar", fmt.Sprintf("%d", s.opts.SampleRate),
		"-ac", fmt.Sprintf("%d", s.opts.ChannelCount),
		"pipe:1",
	)

	for i := 0; i < antiPop; i++ {
		buf.WriteByte(0)
	}

	cmd.Stdout = buf
	cmd.Stderr = os.Stderr

	log.Printf("Running %q %q", s.ffmpeg, cmd.Args)

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}

	for i := 0; i < antiPop; i++ {
		buf.WriteByte(0)
	}

	return buf.Bytes(), nil
}

func (s *Sound) loadSound(filename string) (*oto.Player, error) {
	dir, _ := filepath.Split(filename)
	if dir == "" {
		filename = s.cfg.Asset(filename)
	}

	b, err := s.decodeWithFFmpeg(filename)
	if err != nil {
		return nil, fmt.Errorf("decode error: %w", err)
	}

	s.Lock()
	defer s.Unlock()
	return s.ctx.NewPlayer(bytes.NewReader(b)), nil
}

func (s *Sound) Play(filename string, volume int) (err error) {
	if s.ctx == nil {
		return fmt.Errorf("sound system is disabled")
	}

	s.Lock()
	p, ok := s.sounds[filename]
	s.Unlock()

	if !ok {
		log.Printf("loading %q", filename)
		p, err = s.loadSound(filename)
		if err != nil {
			log.Printf("error loading sound %q: %s", filename, err)
			return
		}
		p.SetVolume(1)
		s.Lock()
		s.sounds[filename] = p
		s.Unlock()
	}

	log.Printf("playing back %q", filename)
	if p.IsPlaying() {
		p.Pause()
		time.Sleep(time.Millisecond * 50)
	}
	_, err = p.Seek(0, 0)
	if err != nil {
		log.Printf("seek failed: %s", err)
	}
	p.SetVolume(0)
	p.Play()
	maxVol := 0.01 * float64(volume)
	if maxVol > 1.0 {
		maxVol = 1.0
	} else if maxVol < 0.0 {
		maxVol = 0.0
	}
	for v := 0.0; v <= maxVol; v += maxVol / 20 {
		p.SetVolume(v)
		time.Sleep(time.Millisecond * 10)
		if maxVol == 0.0 {
			break
		}
	}
	err = p.Err()
	if err != nil {
		log.Printf("play error: %s", err)
	}
	return
}

// vim: ai:ts=8:sw=8:noet:syntax=go
