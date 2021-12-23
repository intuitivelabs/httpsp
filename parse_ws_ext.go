// Copyright 2021 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

package httpsp

import (
	//	"fmt"
	"github.com/intuitivelabs/bytescase"
)

// WSExtT is the type for the web socket extension converted to a flag value
type WSExtT uint

// Sec-WebSocket-Extensions flag values, see
// https://www.iana.org/assignments/websocket/websocket.xhtml#extension-name
const (
	WSExtNone         WSExtT = 0
	WSExtPMsgDeflateF WSExtT = 1 << iota
	WSExtOtherF              // unknown/other
)

// WSExtResolve will try to resolve the extension name to a numeriv WSExtT
// flag.
func WSExtResolve(n []byte) WSExtT {
	if len(n) == 18 && bytescase.CmpEq(n, []byte("permessage-deflate")) {
		return WSExtPMsgDeflateF
	}
	return WSExtOtherF
}

// WSExtVal contains a parsed "Sec-WebSocket-Extension" value.
type WSExtVal struct {
	Val PToken // extension token
	Ext WSExtT // parsed numeric extension value
}

// Reset  re-initializes the internal parsed token.
func (v *WSExtVal) Reset() {
	v.Val.Reset()
	v.Ext = WSExtNone
}

// PWSExt contains the parsed Sec-WebSocket-Extensions header values for one
// or more  different headers (all the  websocket extensions in the message
// that fit in the parsed value array).
type PWSExt struct {
	Vals       []WSExtVal // parsed extension tokens
	N          int        // no of  _values_ found, can be >len(Vals)
	HNo        int        // no of different ws extensions _headers_ found
	Extensions WSExtT     // flags for known extensions
	// parsed hdr content during the last ParseAll... call
	// (contains a trimmed value or several values, overwritten by each
	//  ParseAll.. call; it can also be empty, e.g. on ErrHdrMoreByte)
	LastParsed PField
	tmp        WSExtVal // temporary saved state (between calls)
	first      WSExtVal // even if Vals is nil, we remember the first val.
}

// VNo returns the number of parsed Sec-WebSocket-Extensions headers.
func (u *PWSExt) VNo() int {
	if u.N > len(u.Vals) {
		return len(u.Vals)
	}
	return u.N
}

// GetExt returns the requested parsed extension value or nil.
func (u *PWSExt) GetExt(n int) *WSExtVal {
	if u.VNo() > n {
		return &u.Vals[n]
	}
	if u.Empty() {
		return nil
	}
	if n == 0 {
		return &u.first
	}
	return nil
}

// More returns true if there are more values that did not fit in Vals.
func (u *PWSExt) More() bool {
	return u.N > len(u.Vals)
}

// Reset re-initializes the parsed values.
func (u *PWSExt) Reset() {
	for i := 0; i < u.VNo(); i++ {
		u.Vals[i].Reset()
	}
	v := u.Vals
	*u = PWSExt{}
	u.Vals = v
}

// Init initializes the parsed extensions values buf from an array.
func (u *PWSExt) Init(valbuf []WSExtVal) {
	u.Vals = valbuf
}

// Empty returns true if no extensions values have been parsed.
func (u *PWSExt) Empty() bool {
	return u.N == 0
}

// Parsed returns true if there are some parsed extension values.
func (u *PWSExt) Parsed() bool {
	return u.N > 0
}

// ParseAllWSExtValues tries to parse all the values in a
// Sec-WebSocket-Extensions header situated at offs in buf and adds them to
// the passed PWSExt values.
// The return values are: a new offset after the parsed value (that can be
// used to continue parsing), the number of header values parsed and an error.
// It can return ErrHdrMoreBytes if more data is needed (the value is not
// fully contained in buf).
func ParseAllWSExtValues(buf []byte, offs int, u *PWSExt) (int, int, ErrorHdr) {
	// parsing token list flags
	const flags = PTokCommaSepF | PTokAllowParamsF

	var next int
	var err ErrorHdr
	var pv *WSExtVal

	vNo := 0             // number of values parsed during the current call
	u.LastParsed.Reset() // clear LastParsed on each call
	for {
		if u.N < len(u.Vals) {
			pv = &u.Vals[u.N]
		} else {
			pv = &u.tmp
		}
		next, err = ParseTokenLst(buf, offs, &pv.Val, flags)
		switch err {
		case 0, ErrHdrMoreValues:
			if vNo == 0 {
				u.LastParsed = pv.Val.V
			} else {
				u.LastParsed.Extend(int(pv.Val.V.Offs + pv.Val.V.Len))
			}
			pv.Ext = WSExtResolve(pv.Val.V.Get(buf))
			u.Extensions |= pv.Ext
			vNo++
			u.N++ // next value, continue parsing
			if u.N == 1 && len(u.Vals) == 0 {
				u.first = *pv //set u.first
			}
			if pv == &u.tmp {
				u.tmp.Reset() // prepare for next value (cleanup tmp state)
			}
			if err == ErrHdrMoreValues {
				offs = next
				continue // get next value
			}
		case ErrHdrMoreBytes:
			// do nothing, just for readability
		default:
			pv.Reset() // some error -> clear the crt tmp state
		}
		break
	}
	return next, vNo, err
}
