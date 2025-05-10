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
	"bytes"
)

var freqs []int = []int{
	14473691, 1147017, 167522, 3831121, 356579, 3811315,
	178254, 199644, 183511, 225716, 211240, 308829,
	172852, 186608, 215921, 168891, 168603, 218586,
	284414, 161833, 196043, 151029, 173932, 218370,
	934121, 220530, 381211, 185456, 194675, 161977,
	186680, 182071, 6421956, 537786, 514019, 487155,
	493925, 503143, 514019, 453520, 454241, 485642,
	422407, 593387, 458130, 343687, 342823, 531592,
	324890, 333388, 308613, 293776, 258918, 259278,
	377105, 267488, 227516, 415997, 248763, 301555,
	220962, 206990, 270369, 231694, 273826, 450928,
	384380, 504728, 221251, 376961, 232990, 312574,
	291688, 280236, 252436, 229461, 294353, 241201,
	366590, 199860, 257838, 225860, 260646, 187256,
	266552, 242641, 219450, 192082, 182071, 2185930,
	157439, 164353, 161401, 187544, 186248, 3338637,
	186968, 172132, 148509, 177749, 144620, 192442,
	169683, 209439, 209439, 259062, 194531, 182359,
	159096, 145196, 128199, 158376, 171412, 243433,
	345704, 156359, 145700, 157007, 232342, 154198,
	140730, 288807, 152830, 151246, 250203, 224420,
	161761, 714383, 8188576, 802537, 119484, 123805,
	5632671, 305156, 105584, 105368, 99246, 90459,
	109473, 115379, 261223, 105656, 124381, 100326,
	127550, 89739, 162481, 100830, 97229, 78864,
	107240, 84409, 265760, 116891, 73102, 75695,
	93916, 106880, 86786, 185600, 608367, 133600,
	75695, 122077, 566955, 108249, 259638, 77063,
	166586, 90387, 87074, 84914, 130935, 162409,
	85922, 93340, 93844, 87722, 108249, 98598,
	95933, 427593, 496661, 102775, 159312, 118404,
	114947, 104936, 154342, 140082, 115883, 110769,
	161112, 169107, 107816, 142747, 279804, 85922,
	116315, 119484, 128559, 146204, 130215, 101551,
	91756, 161184, 236375, 131872, 214120, 88875,
	138570, 211960, 94060, 88083, 94564, 90243,
	106160, 88659, 114514, 95861, 108753, 124165,
	427016, 159384, 170547, 104431, 91395, 95789,
	134681, 95213, 105944, 94132, 141883, 102127,
	101911, 82105, 158448, 102631, 87938, 139290,
	114658, 95501, 161329, 126542, 113218, 123661,
	101695, 112930, 317976, 85346, 101190, 189849,
	105728, 186824, 92908, 160896,
}

const MAX_FREQ = 0xffffffff

var table = make([]string, 256)
var rmap = make(map[string]byte)
var tree = make([]*node, 257)
var tree_root *node

type node struct {
	freq      int
	val       byte
	zero, one *node
}

func init() {
	for i, f := range freqs {
		tree[i] = &node{
			freq: f,
			val:  byte(i),
		}
	}

	min_idx1 := -1
	for i := 0; i < 255; i++ {
		min_idx1 = 256
		min_idx2 := 256
		min_freq1 := MAX_FREQ
		min_freq2 := MAX_FREQ

		for j, node := range tree {
			if node == nil {
				continue
			}

			if node.freq < min_freq1 {
				min_idx2, min_freq2 = min_idx1, min_freq1
				min_idx1, min_freq1 = j, node.freq
			} else if node.freq < min_freq2 {
				min_idx2, min_freq2 = j, node.freq
			}
		}

		tree[min_idx1] = &node{
			freq: min_freq1 + min_freq2,
			zero: tree[min_idx2],
			one:  tree[min_idx1],
		}
		tree[min_idx2] = nil
	}

	tree_root = tree[min_idx1]
	fill_table(tree_root, "")
}

func fill_table(node *node, prefix string) {
	if node.zero != nil {
		fill_table(node.zero, prefix+"0")
		fill_table(node.one, prefix+"1")
	} else {
		table[node.val] = prefix
		rmap[prefix] = node.val
	}
}

func HuffmanEncode(data []byte) []byte {
	sb := &bytes.Buffer{}
	bits := 0

	for _, b := range data {
		sb.WriteString(table[b])
		bits += len(table[b])
	}

	b := &bytes.Buffer{}
	chunk_buf := make([]byte, 8)
	for sb.Len() > 0 {
		n, err := sb.Read(chunk_buf)
		if err != nil {
			panic(err)
		}
		chunk := chunk_buf[0:n]
		v := byte(0)
		for i, d := range chunk {
			if d == byte('1') {
				v |= 1 << i
			}
		}
		b.WriteByte(v)
	}

	if len(data) <= b.Len() {
		return append([]byte{0xff}, data...)
	}

	return append([]byte{byte((8 - (bits % 8)) % 8)}, b.Bytes()...)
}

func HuffmanDecode(data []byte) []byte {
	pad := data[0]
	data = data[1:]
	if pad == 0xff {
		return data
	}

	sb := &bytes.Buffer{}
	for _, b := range data {
		for i := 0; i < 8; i++ {
			if (b & (1 << i)) > 0 {
				sb.WriteByte(byte('1'))
			} else {
				sb.WriteByte(byte('0'))
			}
		}
	}

	sb.Truncate(sb.Len() - int(pad))

	b := &bytes.Buffer{}
	node := tree_root
	for _, d := range sb.Bytes() {
		if d == byte('0') && node.zero != nil {
			node = node.zero
		} else if d == byte('1') && node.one != nil {
			node = node.one
		} else {
			b.WriteByte(node.val)
			if d == byte('0') {
				node = tree_root.zero
			} else {
				node = tree_root.one
			}
		}
	}
	if node != tree_root {
		b.WriteByte(node.val)
	}

	return b.Bytes()
}

// vim: ai:ts=8:sw=8:noet:syntax=go
