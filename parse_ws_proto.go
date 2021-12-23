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

// WSProtoT is the type for the web socket protocol converted to a flag value
type WSProtoT uint

// Sec-WebSocket-Protocol flag values, see
// https://www.iana.org/assignments/websocket/websocket.xhtml#subprotocol-name
const (
	WSProtoNone WSProtoT = 0
	WSProtoSIPF WSProtoT = 1 << iota
	WSProtoXMPPF
	WSProtoMSRPF
	WSProtoOtherF // unknown/other
)

// WSProtoResolve will try to resolve the protocol name to a numeriv WSProtoT
// flag.
func WSProtoResolve(n []byte) WSProtoT {
	// few values so faster then using a hash table
	if len(n) == 3 && bytescase.CmpEq(n, []byte("sip")) {
		return WSProtoSIPF
	} else if len(n) == 4 {
		if bytescase.CmpEq(n, []byte("xmpp")) {
			return WSProtoXMPPF
		} else if bytescase.CmpEq(n, []byte("msrp")) {
			return WSProtoMSRPF
		}
	}
	return WSProtoOtherF
}

// WSProtoVal contains a parsed "Sec-WebSocket-Protocol" value.
type WSProtoVal struct {
	Val   PToken   // protocol token
	Proto WSProtoT // parsed numeric protocol value
}

// Reset  re-initializes the internal parsed token.
func (v *WSProtoVal) Reset() {
	v.Val.Reset()
	v.Proto = WSProtoNone
}

// PWSProto contains the parsed Sec-WebSocket-Protocol header values for one
// or more  different headers (all the  websocket sub-protocols in the message
// that fit in the parsed value array).
type PWSProto struct {
	Vals   []WSProtoVal // parsed protocol tokens
	N      int          // no of  _values_ found, can be >len(Vals)
	HNo    int          // no of different ws protocol _headers_ found
	Protos WSProtoT     // flags for known protocols
	// parsed hdr content during the last ParseAll... call
	// (contains a trimmed value or several values, overwritten by each
	//  ParseAll.. call; it can also be empty, e.g. on ErrHdrMoreByte)
	LastParsed PField
	tmp        WSProtoVal // temporary saved state (between calls)
	first      WSProtoVal // even if Vals is nil, we remember the first val.
}

// VNo returns the number of parsed Sec-WebSocket-Protocol headers.
func (u *PWSProto) VNo() int {
	if u.N > len(u.Vals) {
		return len(u.Vals)
	}
	return u.N
}

// GetProto returns the requested parsed proto value or nil.
func (u *PWSProto) GetProto(n int) *WSProtoVal {
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
func (u *PWSProto) More() bool {
	return u.N > len(u.Vals)
}

// Reset re-initializes the parsed values.
func (u *PWSProto) Reset() {
	for i := 0; i < u.VNo(); i++ {
		u.Vals[i].Reset()
	}
	v := u.Vals
	*u = PWSProto{}
	u.Vals = v
}

// Init initializes the parsed proto values buf from an array.
func (u *PWSProto) Init(valbuf []WSProtoVal) {
	u.Vals = valbuf
}

// Empty returns true if no protocol values have been parsed.
func (u *PWSProto) Empty() bool {
	return u.N == 0
}

// Parsed returns true if there are some parsed protocol values.
func (u *PWSProto) Parsed() bool {
	return u.N > 0
}

// ParseAllWSProtoValues tries to parse all the values in a
// Sec-WebSocket-Protocol header situated at offs in buf and adds them to
// the passed PWSProto values.
// The return values are: a new offset after the parsed value (that can be
// used to continue parsing), the number of header values parsed and an error.
// It can return ErrHdrMoreBytes if more data is needed (the value is not
// fully contained in buf).
// Note that this function doesn't try to make any distinction between the
// client & server roles (a client can have a list of tokens or several
// Sec-WebSocket-Protocol headers while the server can have only one value).
func ParseAllWSProtoValues(buf []byte, offs int, u *PWSProto) (int, int, ErrorHdr) {
	const flags = PTokCommaSepF // parsing token list flags
	var next int
	var err ErrorHdr
	var pv *WSProtoVal

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
			pv.Proto = WSProtoResolve(pv.Val.V.Get(buf))
			u.Protos |= pv.Proto
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
