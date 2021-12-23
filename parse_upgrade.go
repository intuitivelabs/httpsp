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

// UpgProtoT is the type for the upgrade protocol converted to a numeric flag
type UpgProtoT uint

// Upgrade protocol flags values, see
//  https://www.iana.org/assignments/http-upgrade-tokens/http-upgrade-tokens.xhtml#http-upgrade-tokens-1
const (
	UProtoNone   UpgProtoT = 0
	UProtoWSockF UpgProtoT = 1 << iota
	UProtoHTTP2F
	UProtoOtherF // unknown/other
)

// UpgProtoGet will try to resolve the protocol name to a numeriv UpgProtoT
// flag.
func UpgProtoResolve(n []byte) UpgProtoT {
	if len(n) == 9 && bytescase.CmpEq(n, []byte("websocket")) {
		return UProtoWSockF
	} else if len(n) == 3 && bytescase.CmpEq(n, []byte("h2c")) {
		return UProtoHTTP2F
	} else if len(n) == 8 && bytescase.CmpEq(n, []byte("http/2.0")) {
		return UProtoHTTP2F
	}
	return UProtoOtherF
}

// UpgProtoVal contains a parsed "Upgrade" protocol value.
type UpgProtoVal struct {
	Val   PToken    // protocol token
	Proto UpgProtoT // parsed numeric protocol value
}

// Reset  re-initializes the internal parsed token.
func (v *UpgProtoVal) Reset() {
	v.Val.Reset()
	v.Proto = UProtoNone
}

// PUpgrade contains the parsed Upgrade header values for one or more
// different Upgrade headers (all the upgrade protocols in the message that
// fit in the parsed value array).
type PUpgrade struct {
	Vals       []UpgProtoVal // parsed protocol tokens
	N          int           // no of upgrade _values_ found, can be >len(Vals)
	HNo        int           // no of different Upgrade: _headers_ found
	Protos     UpgProtoT     // flags for known protocols
	LastParsed PField        // value part of the last Upgrade _header_ parsed
	tmp        UpgProtoVal   // temporary saved state (between calls)
	first      UpgProtoVal   // even if Vals is nil, we remember the first val.
}

// VNo returns the number of parsed Upgrade headers.
func (u *PUpgrade) VNo() int {
	if u.N > len(u.Vals) {
		return len(u.Vals)
	}
	return u.N
}

// GetProto returns the requested parsed upgrade proto value or nil.
func (u *PUpgrade) GetProto(n int) *UpgProtoVal {
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
func (u *PUpgrade) More() bool {
	return u.N > len(u.Vals)
}

// Reset re-initializes the parsed values.
func (u *PUpgrade) Reset() {
	for i := 0; i < u.VNo(); i++ {
		u.Vals[i].Reset()
	}
	v := u.Vals
	*u = PUpgrade{}
	u.Vals = v
}

// Init initializes the parsed proto values buf from an array.
func (u *PUpgrade) Init(valbuf []UpgProtoVal) {
	u.Vals = valbuf
}

// Empty returns true if no upgrade protocol values have been parsed.
func (u *PUpgrade) Empty() bool {
	return u.N == 0
}

// Parsed returns true if there are some parsed upgrade protocol values.
func (u *PUpgrade) Parsed() bool {
	return u.N > 0
}

// ParseAllUpgradeValues tries to parse all the values in an Upgrade header
// situated at offs in buf and adds them to the passed PUpgrade values.
// The return values are: a new offset after the parsed value (that can be
// used to continue parsing), the number of header values parsed and an error.
// It can return ErrHdrMoreBytes if more data is needed (the value is not
// fully contained in buf).
func ParseAllUpgradeValues(buf []byte, offs int, u *PUpgrade) (int, int, ErrorHdr) {
	const flags = PTokCommaSepF | PTokAllowSlashF // parsing token list flags
	var next int
	var err ErrorHdr
	var pv *UpgProtoVal

	vNo := 0             // number of values parsed during the current call
	u.LastParsed.Reset() // clear LastParsed on each call
	for {
		if u.N < len(u.Vals) {
			pv = &u.Vals[u.N]
		} else {
			pv = &u.tmp
		}
		next, err = ParseTokenLst(buf, offs, &pv.Val, flags)
		/*
			fmt.Printf("ParseToken(%q, (%d), %p) -> %d, %q  rest %q\n",
				buf[offs:], offs, pf, next, err, buf[next:])
		*/
		switch err {
		case 0, ErrHdrMoreValues:
			if vNo == 0 {
				u.LastParsed = pv.Val.V
			} else {
				u.LastParsed.Extend(int(pv.Val.V.Offs + pv.Val.V.Len))
			}
			pv.Proto = UpgProtoResolve(pv.Val.V.Get(buf))
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
