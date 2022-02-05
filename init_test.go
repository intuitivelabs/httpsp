// Copyright 2021 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE_BSD.txt file in the root of the source
// tree.

package httpsp

// Init functions for testing

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"
)

var seed int64 // rand() seed

func TestMain(m *testing.M) {
	seed = time.Now().UnixNano()
	flag.Int64Var(&seed, "seed", seed, "random seed")
	flag.Parse()
	rand.Seed(seed)
	fmt.Printf("using random seed %d (0x%x) ( \"-seed\" to change)\n",
		seed, seed)
	res := m.Run()
	os.Exit(res)
}
