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
	"sync"
	"time"
)

type Alerter struct {
	lastEvent *AlertEvent
	mu        *sync.Mutex
	cv        *sync.Cond
	Sound     *Sound
}

type AlertEvent struct {
	Text  string `json:"text"`
	Image string `json:"image,omitempty"`
	Sound string `json:"sound,omitempty"`
}

func NewAlerter() *Alerter {
	a := &Alerter{}
	a.mu = new(sync.Mutex)
	a.cv = sync.NewCond(a.mu)
	return a
}

func (a *Alerter) Broadcast(event AlertEvent) {
	a.mu.Lock()
	a.lastEvent = &event
	a.mu.Unlock()

	a.cv.Broadcast()

	if event.Sound != "" {
		a.Sound.Play(event.Sound)
	}
}

func (a *Alerter) Subscribe() <-chan AlertEvent {
	ch := make(chan AlertEvent)
	go func(ch chan AlertEvent) {
		a.mu.Lock()
		last_event := a.lastEvent
		a.mu.Unlock()

		running := true
		for running {
			a.mu.Lock()
			var event *AlertEvent
			for {
				event = a.lastEvent
				if event != nil && event != last_event {
					break
				}
				a.cv.Wait()
			}
			a.mu.Unlock()

			last_event = event
			t := time.NewTimer(time.Second)
			select {
			case <-t.C:
				// timed out
				running = false
			case ch <- *event:
				// done
			}
			t.Stop()
		}
	}(ch)
	return ch
}

// vim: ai:ts=8:sw=8:noet:syntax=go
