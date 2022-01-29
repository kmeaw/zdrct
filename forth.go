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
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/anko/env"
	"github.com/mattn/anko/vm"
)

type Mode int

const (
	MODE_NORMAL Mode = iota
	MODE_IF
	MODE_LITSTR
	MODE_LOOP
)

var ErrBreak = fmt.Errorf("break")

func (b *IRCBot) EvalForth(ctx context.Context, e *env.Env, tokens ...string) (interface{}, error) {
	mode := MODE_NORMAL
	next_mode := MODE_NORMAL
	if_level := 0

	stack := []interface{}{}
	counters := []int64{}
	loop_tokens := []string{}
	litstr := ""
	for _, token := range tokens {
		next_mode = mode
		switch mode {
		case MODE_IF:
			switch token {
			case "if":
				if_level += 1
			case "then":
				if_level -= 1
				if if_level == 0 {
					next_mode = MODE_NORMAL
				}
			}
		case MODE_LITSTR:
			if strings.HasSuffix(token, "\"") {
				litstr = litstr + strings.TrimSuffix(token, "\"")
				stack = append([]interface{}{litstr}, stack...)
				next_mode = MODE_NORMAL
			} else {
				litstr = litstr + token + " "
			}
		case MODE_LOOP:
			if token == "loop" {
				var out interface{}
				var err error

				counter := counters[0]
				counters = counters[1:]
				for i := int64(0); i < counter; i++ {
					out, err = b.EvalForth(ctx, e, loop_tokens...)
					if err != nil {
						if err == ErrBreak {
							break
						} else {
							return nil, err
						}
					}
				}
				stack = append([]interface{}{out}, stack...)
				next_mode = MODE_NORMAL
			} else {
				loop_tokens = append(loop_tokens, token)
			}
		}
		if mode != 0 {
			mode = next_mode
			continue
		}

		switch token {
		case "":
			// no-op
		case "drop":
			if len(stack) == 0 {
				return nil, fmt.Errorf("stack underflow")
			}
			stack = stack[1:]
		case "dup":
			if len(stack) == 0 {
				return nil, fmt.Errorf("stack underflow")
			}
			stack = append([]interface{}{stack[0]}, stack...)
		case "if":
			if len(stack) == 0 {
				return nil, fmt.Errorf("stack underflow")
			}
			if_level += 1
			top := stack[0]
			stack = stack[1:]
			if top == 0 || top == nil || top == "" {
				mode = MODE_IF
			}
		case "then":
			if if_level > 0 {
				if_level -= 1
			} else {
				return nil, fmt.Errorf("unexpected then")
			}
		case "not":
			if len(stack) == 0 {
				return nil, fmt.Errorf("stack underflow")
			}
			top := stack[0]
			stack = stack[1:]
			if top == 0 || top == nil || top == "" {
				stack = append([]interface{}{true}, stack[1:]...)
			}
		case "reply":
			if len(stack) == 0 {
				return nil, fmt.Errorf("stack underflow")
			}
			top := stack[0]
			stack = stack[1:]
			b.mu.Lock()
			t0 := time.Now()
			t1 := b.LastBuckets["reply"]
			if t0.After(t1) {
				b.Reply("%s", top)
				b.LastBuckets["reply"] = time.Now().Add(time.Second)
			}
			b.mu.Unlock()
		case "\"":
			mode = MODE_LITSTR
		case "times":
			if len(stack) == 0 {
				return nil, fmt.Errorf("stack underflow")
			}
			top := stack[0]
			stack = stack[1:]
			counter, ok := top.(int64)
			if !ok {
				return nil, fmt.Errorf("type error: %T, expected int64", top)
			}

			if counter < 0 || counter > 100 {
				return nil, fmt.Errorf("times domain error: %d", counter)
			}

			counters = append([]int64{counter}, counters...)
			mode = MODE_LOOP
		case "+":
			if len(stack) < 2 {
				return nil, fmt.Errorf("stack underflow")
			}
			a, ok := stack[0].(int64)
			if !ok {
				return nil, fmt.Errorf("type error: %T is not an integer", stack[0])
			}
			b, ok := stack[1].(int64)
			if !ok {
				return nil, fmt.Errorf("type error: %T is not an integer", stack[1])
			}
			stack = append([]interface{}{a + b}, stack[2:])
		case ">":
			if len(stack) < 2 {
				return nil, fmt.Errorf("stack underflow")
			}
			a, ok := stack[0].(int64)
			if !ok {
				return nil, fmt.Errorf("type error: %T is not an integer", stack[0])
			}
			b, ok := stack[1].(int64)
			if !ok {
				return nil, fmt.Errorf("type error: %T is not an integer", stack[1])
			}
			stack = append([]interface{}{a > b}, stack[2:])
		case "=":
			if len(stack) < 2 {
				return nil, fmt.Errorf("stack underflow")
			}
			a := fmt.Sprintf("%v", stack[0])
			b := fmt.Sprintf("%v", stack[1])
			stack = append([]interface{}{a == b}, stack[2:])
		default:
			if token[0] == '!' {
				cmd_tokens := strings.Split(token[1:], ";")
				if len(cmd_tokens) == 0 {
					return nil, fmt.Errorf("cannot tokenize empty string")
				}
				args := make([]string, 0, len(cmd_tokens)-1)
				for _, arg := range cmd_tokens[1:] {
					args = append(args, fmt.Sprintf("%q", arg))
				}
				script := fmt.Sprintf("cmd_%s(%s)", cmd_tokens[0], strings.Join(args, ", "))
				result, err := vm.ExecuteContext(ctx, e, nil, script)
				if err != nil {
					log.Printf("cannot execute script %q: %s", script, err)
					return nil, err
				} else {
					log.Printf("script by %s: %s", ctx.Value("from_user"), script)
				}
				stack = append([]interface{}{result}, stack...)
			} else {
				n, err := strconv.ParseInt(token, 0, 64)
				if err != nil {
					return nil, fmt.Errorf("unrecognized token: %q", token)
				}

				stack = append([]interface{}{n}, stack...)
			}
		}
	}

	if len(stack) > 0 {
		return stack[0], nil
	} else {
		return nil, nil
	}
}

// vim: ai:ts=8:sw=8:noet:syntax=go
