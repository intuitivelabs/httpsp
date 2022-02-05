// Copyright 2019-2020 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

// Test utils

package httpsp

import (
	"github.com/intuitivelabs/bytescase"
	"math/rand"
)

func randWS() string {
	ws := [...]string{"", " ", "	"}
	var s string
	n := rand.Intn(5) // max 5 whitespace "tokens"
	for i := 0; i < n; i++ {
		s += ws[rand.Intn(len(ws))]
	}
	return s
}

func randLWS() string {
	ws := [...]string{
		"", " ", "  ", "\r\n ", "\r\n   ", "\n ", "\r ",
	}
	var s string
	n := rand.Intn(5) // max 5 whitespace "tokens"
	for i := 0; i < n; i++ {
		s += ws[rand.Intn(len(ws))]
	}
	return s
}

// randomize case in a string
func randCase(s string) string {
	r := make([]byte, len(s))
	for i, b := range []byte(s) {
		switch rand.Intn(3) {
		case 0:
			r[i] = bytescase.ByteToLower(b)
		case 1:
			r[i] = bytescase.ByteToUpper(b)
		default:
			r[i] = b
		}
	}
	return string(r)
}

// simple slow unescape \n, \r, \tm \\
// returns allocated byte slice
func unescapeCRLF(s string) []byte {
	buf := make([]byte, len(s)+2) // space for extra \r\n
	// handle escapes: \r & \n
	var i int
	var escape bool
	for _, b := range []byte(s) {
		if b == '\\' && escape == false {
			escape = true
			continue
		}
		if escape {
			switch b {
			case 'n':
				buf[i] = '\n'
			case 'r':
				buf[i] = '\r'
			case 't':
				buf[i] = '\t'
			case '\\':
				buf[i] = '\\'
			default:
				// unrecognized => \char
				buf[i] = '\\'
				i++
				buf[i] = b
			}
			escape = false
		} else {
			buf[i] = b
		}
		i++
	}
	buf = buf[:i]
	return buf
}
