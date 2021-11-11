// Copyright 2019-2020 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

// Test utils

package httpsp

import (
	"math/rand"
)

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
