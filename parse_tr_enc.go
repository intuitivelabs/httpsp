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

// TrEncT is the type for the web socket extension converted to a flag value
type TrEncT uint

// Transfer-Encoding flag values, see
// https://www.rfc-editor.org/rfc/rfc7230#section-4 and
// http://www.iana.org/assignments/http-parameters/http-parameters.xhtml#transfer-coding
const (
	TrEncNone     TrEncT = 0
	TrEncChunkedF TrEncT = 1 << iota
	TrEncCompressF
	TrEncDeflateF
	TrEncGzipF
	TrEncIdentityF
	TrEncTrailersF  // not an actual encoding, used in TE
	TrEncXCompressF // obsolete
	TrEncXGzipF     // obsolete
	TrEncOtherF     // unknown/other
)

// TrEncResolve will try to resolve the extension name to a numeriv TrEncT
// flag.
func TrEncResolve(n []byte) TrEncT {
	switch len(n) {
	case 7:
		if bytescase.CmpEq(n, []byte("chunked")) {
			return TrEncChunkedF
		} else if bytescase.CmpEq(n, []byte("deflate")) {
			return TrEncDeflateF
		}
	case 8:
		if bytescase.CmpEq(n, []byte("compress")) {
			return TrEncCompressF
		} else if bytescase.CmpEq(n, []byte("identity")) {
			return TrEncIdentityF
		} else if bytescase.CmpEq(n, []byte("trailers")) {
			return TrEncTrailersF
		}
	case 4:
		if bytescase.CmpEq(n, []byte("gzip")) {
			return TrEncGzipF
		}
	case 10:
		if bytescase.CmpEq(n, []byte("x-compress")) {
			return TrEncXCompressF
		}
	case 6:
		if bytescase.CmpEq(n, []byte("x-gzip")) {
			return TrEncXGzipF
		}
	}
	return TrEncOtherF
}

// TrEncVal contains a parsed "Transfer-Encoding" or "TE" value.
type TrEncVal struct {
	Val PToken // encoding as token
	Enc TrEncT // parsed numeric encoding flag
}

// Reset  re-initializes the internal parsed token.
func (v *TrEncVal) Reset() {
	v.Val.Reset()
	v.Enc = TrEncNone
}

// PTrEnc contains the parsed Transfer-Encoding header values for one
// or more  different headers (all the  transfer encodings in the message
// that fit in the parsed value array).
type PTrEnc struct {
	Vals      []TrEncVal // parsed encoding tokens
	N         int        // no of  _values_ found, can be >len(Vals)
	HNo       int        // no of different encoding _headers_ found
	Encodings TrEncT     // flags for known encodings
	// parsed hdr content during the last ParseAll... call
	// (contains a trimmed value or several values, overwritten by each
	//  ParseAll.. call; it can also be empty, e.g. on ErrHdrMoreByte)
	LastParsed PField
	First      TrEncVal // even if Vals is nil, we remember the first val.
	Last       TrEncVal // last parsed value
	tmp        TrEncVal // temporary saved state (between calls)
}

// VNo returns the number of parsed Transfer-Encoding headers.
func (u *PTrEnc) VNo() int {
	if u.N > len(u.Vals) {
		return len(u.Vals)
	}
	return u.N
}

// GetExt returns the requested parsed extension value or nil.
func (u *PTrEnc) GetExt(n int) *TrEncVal {
	if u.VNo() > n {
		return &u.Vals[n]
	}
	if u.Empty() {
		return nil
	}
	if n == 0 {
		return &u.First
	} else if n == (u.VNo() - 1) {
		return &u.Last
	}
	return nil
}

// More returns true if there are more values that did not fit in Vals.
func (u *PTrEnc) More() bool {
	return u.N > len(u.Vals)
}

// Reset re-initializes the parsed values.
func (u *PTrEnc) Reset() {
	for i := 0; i < u.VNo(); i++ {
		u.Vals[i].Reset()
	}
	v := u.Vals
	*u = PTrEnc{}
	u.Vals = v
}

// Init initializes the parsed extensions values buf from an array.
func (u *PTrEnc) Init(valbuf []TrEncVal) {
	u.Vals = valbuf
}

// Empty returns true if no extensions values have been parsed.
func (u *PTrEnc) Empty() bool {
	return u.N == 0
}

// Parsed returns true if there are some parsed extension values.
func (u *PTrEnc) Parsed() bool {
	return u.N > 0
}

// ParseAllTrEncValues tries to parse all the values in a
// Transfer-Encoding header situated at offs in buf and adds them to
// the passed PTrEnc values.
// The return values are: a new offset after the parsed value (that can be
// used to continue parsing), the number of header values parsed and an error.
// It can return ErrHdrMoreBytes if more data is needed (the value is not
// fully contained in buf).
func ParseAllTrEncValues(buf []byte, offs int, u *PTrEnc) (int, int, ErrorHdr) {
	// parsing token list flags (params allowed)
	const flags = PTokCommaSepF | PTokAllowParamsF

	var next int
	var err ErrorHdr
	var pv *TrEncVal

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
			pv.Enc = TrEncResolve(pv.Val.V.Get(buf))
			u.Encodings |= pv.Enc
			vNo++
			u.N++ // next value, continue parsing
			if u.N == 1 && len(u.Vals) == 0 {
				u.First = *pv //set u.First
			}
			u.Last = *pv // always set Last
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
