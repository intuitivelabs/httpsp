// Copyright 2021 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

package httpsp

import (
	"bytes"

	"github.com/intuitivelabs/bytescase"
)

// HTTPMethod is the type used to hold the various SIP request methods.
type HTTPMethod uint8

// method types
const (
	MUndef HTTPMethod = iota
	MGet
	MHead
	MPost
	MPut
	MDelete
	MConnect
	MOptions
	MTrace
	MPatch
	MOther // must be last
)

// Method2Name translates between a numeric HTTPMethod and the ASCII name.
var Method2Name = [MOther + 1][]byte{
	MUndef:   []byte(""),
	MGet:     []byte("GET"),
	MHead:    []byte("HEAD"),
	MPost:    []byte("POST"),
	MPut:     []byte("PUT"),
	MDelete:  []byte("DELETE"),
	MConnect: []byte("CONNECT"),
	MOptions: []byte("OPTIONS"),
	MTrace:   []byte("TRACE"),
	MPatch:   []byte("PATCH"),
	MOther:   []byte("OTHER"),
}

// Name returns the ASCII sip method name.
func (m HTTPMethod) Name() []byte {
	if m > MOther {
		return Method2Name[MUndef]
	}
	return Method2Name[m]
}

// String implements the Stringer interface (converts the method to string,
// similar to Name()).
func (m HTTPMethod) String() string {
	return string(m.Name())
}

// GetMethodNo converts from an ASCII SIP method name to the corresponding
// numeric internal value.
func GetMethodNo(buf []byte) HTTPMethod {
	i := hashMthName(buf)
	for _, m := range mthNameLookup[i] {
		if bytes.Equal(buf, m.n) {
			return m.t
		}
	}
	return MOther
}

// magic values: after adding/removing methods run tests again
// looking for max. elem per bucket == 1 for minimum hash size
const (
	mthBitsLen   uint = 2 //re-run tests after changing
	mthBitsFChar uint = 3
)

type mth2Type struct {
	n []byte
	t HTTPMethod
}

var mthNameLookup [1 << (mthBitsLen + mthBitsFChar)][]mth2Type

func hashMthName(n []byte) int {
	const (
		mC = (1 << mthBitsFChar) - 1
		mL = (1 << mthBitsLen) - 1
	)
	return (int(bytescase.ByteToLower(n[0])) & mC) |
		((len(n) & mL) << mthBitsFChar)
}

func init() {
	// init lookup method-to-type array
	for i := MUndef + 1; i < MOther; i++ {
		h := hashMthName(Method2Name[i])
		mthNameLookup[h] =
			append(mthNameLookup[h], mth2Type{Method2Name[i], i})
	}
}
