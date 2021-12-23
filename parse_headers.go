// Copyright 2021 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

package httpsp

import (
	"github.com/intuitivelabs/bytescase"
)

// HdrT is used to hold the header type as a numeric constant.
type HdrT uint16

// HdrFlags packs several header values into bit flags.
type HdrFlags uint16

// Reset initializes a HdrFlags.
func (f *HdrFlags) Reset() {
	*f = 0
}

// Set sets the header flag corresponding to the passed header type.
func (f *HdrFlags) Set(Type HdrT) {
	*f |= 1 << Type
}

// Clear resets the header flag corresponding to the passed header type.
func (f *HdrFlags) Clear(Type HdrT) {
	*f &^= 1 << Type // equiv to & ^(...)
}

// Test returns true if the flag corresponding to the passed header type
// is set.
func (f HdrFlags) Test(Type HdrT) bool {
	return (f & (1 << Type)) != 0
}

// Any returns true if at least one of the passed header types is set.
func (f HdrFlags) Any(types ...HdrT) bool {
	for _, t := range types {
		if f&(1<<t) != 0 {
			return true
		}
	}
	return false
}

// AllSet returns true if all of the passed header types are set.
func (f HdrFlags) AllSet(types ...HdrT) bool {
	for _, t := range types {
		if f&(1<<t) == 0 {
			return false
		}
	}
	return true
}

// HdrT header types constants.
const (
	HdrNone HdrT = iota
	HdrCLen
	HdrTrEncoding
	HdrUpgrade // http 1.1 _only_ (not allower on 2.0)
	HdrCEncoding
	HdrHost
	HdrServer
	HdrOrigin
	HdrConnection
	HdrWSockKey
	HdrWSockProto
	HdrWSockAccept
	HdrWSockVer
	HdrWSockExt
	HdrOther // generic, not recognized header
)

// HdrFlags constants for each header type.
const (
	HdrCLenF        HdrFlags = 1 << HdrCLen
	HdrTrEncodingF  HdrFlags = 1 << HdrTrEncoding
	HdrUpgradeF     HdrFlags = 1 << HdrUpgrade
	HdrCEncodingF   HdrFlags = 1 << HdrCEncoding
	HdrHostF        HdrFlags = 1 << HdrHost
	HdrServerF      HdrFlags = 1 << HdrServer
	HdrOriginF      HdrFlags = 1 << HdrOrigin
	HdrConnectionF  HdrFlags = 1 << HdrConnection
	HdrWSockKeyF    HdrFlags = 1 << HdrWSockKey
	HdrWSockProtoF  HdrFlags = 1 << HdrWSockProto
	HdrWSockAcceptF HdrFlags = 1 << HdrWSockAccept
	HdrWSockVerF    HdrFlags = 1 << HdrWSockVer
	HdrWSockExtF    HdrFlags = 1 << HdrWSockExt
	HdrOtherF       HdrFlags = 1 << HdrOther
)

// pretty names for debugging and error reporting
var hdrTStr = [...]string{
	HdrNone:        "nil",
	HdrCLen:        "Content-Length",
	HdrTrEncoding:  "Transfer-Encoding",
	HdrUpgrade:     "Upgrade",
	HdrCEncoding:   "Content-Encoding",
	HdrHost:        "Host",
	HdrServer:      "Server",
	HdrOrigin:      "Origin",
	HdrConnection:  "Connection",
	HdrWSockKey:    "Sec-WebSocket-Key",
	HdrWSockProto:  "Sec-WebSocket-Protocol",
	HdrWSockAccept: "Sec-WebSocket-Accept",
	HdrWSockVer:    "Sec-WebSocket-Version",
	HdrWSockExt:    "Sec-WebSocket-Extensions",
	HdrOther:       "Generic",
}

// String implements the Stringer interface.
func (t HdrT) String() string {
	if int(t) >= len(hdrTStr) || int(t) < 0 {
		return "invalid"
	}
	return hdrTStr[t]
}

// associates header name (as byte slice) to HdrT header type
type hdr2Type struct {
	n []byte
	t HdrT
}

// list of header-name <-> header type correspondence
// (always use lowercase)
var hdrName2Type = [...]hdr2Type{
	{n: []byte("content-length"), t: HdrCLen},
	{n: []byte("transfer-encoding"), t: HdrTrEncoding},
	{n: []byte("upgrade"), t: HdrUpgrade},
	{n: []byte("content-encoding"), t: HdrCEncoding},
	{n: []byte("host"), t: HdrHost},
	{n: []byte("server"), t: HdrServer},
	{n: []byte("connection"), t: HdrConnection},
	{n: []byte("sec-websocket-key"), t: HdrWSockKey},
	{n: []byte("sec-websocket-protocol"), t: HdrWSockProto},
	{n: []byte("sec-websocket-accept"), t: HdrWSockAccept},
	{n: []byte("sec-websocket-version"), t: HdrWSockVer},
	{n: []byte("sec-websocket-extensions"), t: HdrWSockExt},
	{n: []byte("origin"), t: HdrOrigin},
}

const (
	hnBitsLen   uint = 2 // after changing this re-run testing
	hnBitsFChar uint = 5
)

var hdrNameLookup [1 << (hnBitsLen + hnBitsFChar)][]hdr2Type

func hashHdrName(n []byte) int {
	// simple hash:
	//           1stchar & mC | (len &mL<< bitsFChar)
	const (
		mC = (1 << hnBitsFChar) - 1
		mL = (1 << hnBitsLen) - 1
	)
	/* contact & callid will have the same hash, using this method...*/
	return (int(bytescase.ByteToLower(n[0])) & mC) |
		((len(n) & mL) << hnBitsFChar)
}

func init() {
	// init lookup arrays
	for _, h := range hdrName2Type {
		i := hashHdrName(h.n)
		hdrNameLookup[i] = append(hdrNameLookup[i], h)
	}
}

// GetHdrType returns the corresponding HdrT type for a given header name.
// The header name should not contain any leading or ending white space.
func GetHdrType(name []byte) HdrT {
	i := hashHdrName(name)
	for _, h := range hdrNameLookup[i] {
		if bytescase.CmpEq(name, h.n) {
			return h.t
		}
	}
	return HdrOther
}

// Hdr contains a partial or fully parsed header.
type Hdr struct {
	Type HdrT
	Name PField
	Val  PField
	HdrIState
}

// Reset re-initializes the parsing state and the parsed values.
func (h *Hdr) Reset() {
	*h = Hdr{}
}

// Missing returns true if the header is empty (not parsed).
func (h *Hdr) Missing() bool {
	return h.Type == HdrNone
}

// HdrIState contains internal header parsing state.
type HdrIState struct {
	state uint8
}

// HdrLst groups a list of parsed headers.
type HdrLst struct {
	PFlags HdrFlags               // parsed headers as flags
	N      int                    // total numbers of headers found (can be > len(Hdrs))
	Hdrs   []Hdr                  // all parsed headers, that fit in the slice.
	h      [int(HdrOther) - 1]Hdr // list of type -> hdr, pointing to the
	// first hdr with the corresponding type.
	HdrLstIState
}

// HdrLstIState contains internal HdrLst parsing state.
type HdrLstIState struct {
	hdr Hdr // tmp. header used for saving the state
}

// Reset re-initializes the parsing state and values.
func (hl *HdrLst) Reset() {
	hdrs := hl.Hdrs
	*hl = HdrLst{}
	for i := 0; i < len(hdrs); i++ {
		hdrs[i].Reset()
	}
	hl.Hdrs = hdrs
}

// GetHdr returns the first parsed header of the requested type.
// If no corresponding header was parsed it returns nil.
func (hl *HdrLst) GetHdr(t HdrT) *Hdr {
	if t > HdrNone && t < HdrOther {
		return &hl.h[int(t)-1] // no value for HdrNone or HdrOther
	}
	return nil
}

// SetHdr adds a new header to the  internal "first" header list (see GetHdr)
// if not already present.
// It returns true if successful and false if a header of the same type was
// already added or the header type is invalid.
func (hl *HdrLst) SetHdr(newhdr *Hdr) bool {
	i := int(newhdr.Type) - 1
	if i >= 0 && i < len(hl.h) && hl.h[i].Missing() {
		hl.h[i] = *newhdr
		return true
	}
	return false
}

// PHBodies defines an interface for getting pointers to parsed bodies structs.
type PHBodies interface {
	GetCLen() *PUIntBody
	GetUpgrade() *PUpgrade
	GetTrEnc() *PTrEnc
	GetWSProto() *PWSProto
	GetWSExt() *PWSExt
	Reset()
}

// PHdrVals holds all the header specific parsed values structures.
// (implements PHBodies)
type PHdrVals struct {
	CLen    PUIntBody
	Upgrade PUpgrade
	TrEnc   PTrEnc
	WSProto PWSProto
	WSExt   PWSExt
}

// Reset re-initializes all the parsed values.
func (hv *PHdrVals) Reset() {
	hv.CLen.Reset()
	hv.Upgrade.Reset()
	hv.TrEnc.Reset()
	hv.WSProto.Reset()
	hv.WSExt.Reset()
}

// GetCLen returns a pointer to the parsed content-length body.
// It implements the PHBodies interface.
func (hv *PHdrVals) GetCLen() *PUIntBody {
	return &hv.CLen
}

// GetUpgrade returns a pointer to the parsed Upgrade body.
// It implements the PHBodies interface.
func (hv *PHdrVals) GetUpgrade() *PUpgrade {
	return &hv.Upgrade
}

// GetTrEnc returns a pointer to the parsed Transfer-Encoding body.
// It implements the PHBodies interface.
func (hv *PHdrVals) GetTrEnc() *PTrEnc {
	return &hv.TrEnc
}

// GetWSProto returns a pointer to the parsed Sec-WebSocket-Protocol body.
// It implements the PHBodies interface.
func (hv *PHdrVals) GetWSProto() *PWSProto {
	return &hv.WSProto
}

// GetWSExt returns a pointer to the parsed Sec-WebSocket-Extensions body.
// It implements the PHBodies interface.
func (hv *PHdrVals) GetWSExt() *PWSExt {
	return &hv.WSExt
}

// ParseHdrLine parses a header from a HTTP message.
// The parameters are: a message buffer, the offset in the buffer where the
// parsing should start (or continue), a pointer to a Hdr structure that will
// be filled and a PHBodies interface (defining methods to obtain pointers to
// from, to, callid, cseq and content-length specific parsed body structures
// that will be filled if one of the corresponding headers is found).
// It returns a new offset, pointing immediately after the end of the header
// (it could point to len(buf) if the header and the end of the buffer
// coincide) and an error. If the first line  is not fully contained in
// buf[offs:] ErrHdrMoreBytes will be returned and this function can be called
// again when more bytes are available, with the same buffer, the returned
// offset ("continue point") and the same Hdr structure.
// Another special error value is ErrHdrEmpty. It is returned if the header
// is empty ( CR LF). If previous headers were parsed, this means the end of
// headers was encountered. The offset returned is after the CRLF.
func ParseHdrLine(buf []byte, offs int, h *Hdr, hb PHBodies) (int, ErrorHdr) {
	// grammar:  Name SP* : LWS* val LWS* CRLF
	const (
		hInit uint8 = iota
		hName
		hNameEnd
		hBodyStart
		hVal
		hValEnd
		hCLen
		hUpgrade
		hTrEncoding
		hWSockProto
		hWSockExt
		hFIN
	)

	// helper internal function for parsing header specific values if
	//  header specific parser are available (else fall back to generic
	//  value parsing)
	parseBody := func(buf []byte, o int, h *Hdr, hb PHBodies) (int, ErrorHdr) {
		var err ErrorHdr
		n := o
		if hb != nil {
			switch h.Type {
			case HdrCLen:
				if clenb := hb.GetCLen(); clenb != nil && !clenb.Parsed() {
					h.state = hCLen
					n, err = ParseCLenVal(buf, o, clenb)
					if err == 0 { /* fix hdr.Val */
						h.Val = clenb.SVal
					}
				}
			case HdrUpgrade:
				if upgrade := hb.GetUpgrade(); upgrade != nil {
					if h.state != hUpgrade {
						// new Upgrade header found
						upgrade.HNo++
					}
					h.state = hUpgrade
					n, _, err = ParseAllUpgradeValues(buf, o, upgrade)
					// fix hdr.Val
					h.Val = upgrade.LastParsed
				}
			case HdrTrEncoding:
				if trEnc := hb.GetTrEnc(); trEnc != nil {
					if h.state != hTrEncoding {
						// new Transfer-Encoding header found
						trEnc.HNo++
					}
					h.state = hTrEncoding
					n, _, err = ParseAllTrEncValues(buf, o, trEnc)
					// fix hdr.Val
					h.Val = trEnc.LastParsed
				}
			case HdrWSockProto:
				if wsProto := hb.GetWSProto(); wsProto != nil {
					if h.state != hWSockProto {
						// new Sec-WebSocket-Protocol header found
						wsProto.HNo++
					}
					h.state = hWSockProto
					n, _, err = ParseAllWSProtoValues(buf, o, wsProto)
					// fix hdr.Val
					h.Val = wsProto.LastParsed
				}
			case HdrWSockExt:
				if wsExt := hb.GetWSExt(); wsExt != nil {
					if h.state != hWSockExt {
						// new Sec-WebSocket-Extensions header found
						wsExt.HNo++
					}
					h.state = hWSockExt
					n, _, err = ParseAllWSExtValues(buf, o, wsExt)
					// fix hdr.Val
					h.Val = wsExt.LastParsed
				}
			}
		}
		return n, err
	}

	var crl int
	i := offs
	for i < len(buf) {
		switch h.state {
		case hInit:
			if (len(buf) - i) < 1 {
				goto moreBytes
			}
			if buf[i] == '\r' {
				if (len(buf) - i) < 2 {
					goto moreBytes
				}
				h.state = hFIN
				if buf[i+1] == '\n' {
					/* CRLF - end of header */
					return i + 2, ErrHdrEmpty
				}
				return i + 1, ErrHdrEmpty // single CR
			} else if buf[i] == '\n' {
				/* single LF, accept it as valid end of header */
				h.state = hFIN
				return i + 1, ErrHdrEmpty
			}
			h.state = hName
			h.Name.Set(i, i)
			fallthrough
		case hName:
			i = skipTokenDelim(buf, i, ':')
			if i >= len(buf) {
				goto moreBytes
			}
			if buf[i] == ' ' || buf[i] == '\t' {
				h.state = hNameEnd
				h.Name.Extend(i)
				if h.Name.Empty() {
					goto errEmptyTok
				}
				i++
			} else if buf[i] == ':' {
				h.state = hBodyStart
				h.Name.Extend(i)
				if h.Name.Empty() {
					goto errEmptyTok
				}
				h.Type = GetHdrType(h.Name.Get(buf))
				i++
				n, err := parseBody(buf, i, h, hb)
				if h.state != hBodyStart {
					if err == 0 {
						h.state = hFIN
					}
					return n, err
				}
			} else {
				// non WS after seeing a token => error
				goto errBadChar
			}
		case hNameEnd:
			i = skipWS(buf, i)
			if i >= len(buf) {
				goto moreBytes
			}
			if buf[i] == ':' {
				h.state = hBodyStart
				h.Type = GetHdrType(h.Name.Get(buf))
				i++
				n, err := parseBody(buf, i, h, hb)
				if h.state != hBodyStart {
					if err == 0 {
						h.state = hFIN
					}
					return n, err
				}
			} else {
				// non WS after seing a token => error
				goto errBadChar
			}
		case hBodyStart:
			var err ErrorHdr
			i, crl, err = skipLWS(buf, i, 0)
			switch err {
			case 0:
				h.state = hVal
				h.Val.Set(i, i)
				crl = 0
			case ErrHdrEOH:
				// empty value
				goto endOfHdr
			case ErrHdrMoreBytes:
				fallthrough
			default:
				return i, err
			}
			i++
		case hVal:
			i = skipToken(buf, i)
			if i >= len(buf) {
				goto moreBytes
			}
			h.Val.Extend(i)
			h.state = hValEnd
			fallthrough
		case hValEnd:
			var err ErrorHdr
			i, crl, err = skipLWS(buf, i, 0)
			switch err {
			case 0:
				h.state = hVal
				crl = 0
			case ErrHdrEOH:
				goto endOfHdr
			case ErrHdrMoreBytes:
				fallthrough
			default:
				return i, err
			}
			i++
		case hCLen: // continue content-length parsing
			clenb := hb.GetCLen()
			n, err := ParseCLenVal(buf, i, clenb)
			if err == 0 { /* fix hdr.Val */
				h.Val = clenb.SVal
				h.state = hFIN
			}
			return n, err
		case hUpgrade: // continue Upgrade parsing (multiple vals possible)
			upgrades := hb.GetUpgrade()
			n, _, err := ParseAllUpgradeValues(buf, i, upgrades)
			// fix hdr. Val
			if h.Val.Empty() {
				h.Val = upgrades.LastParsed
			} else if !upgrades.LastParsed.Empty() {
				// add the last parsed part to current header content
				h.Val.Extend(upgrades.LastParsed.EndOffs())
			}
			if err == 0 {
				h.state = hFIN
			}
			return n, err
		case hTrEncoding: // continue Tr-Enc parsing (multiple vals possible)
			trEnc := hb.GetTrEnc()
			n, _, err := ParseAllTrEncValues(buf, i, trEnc)
			// fix hdr. Val
			if h.Val.Empty() {
				h.Val = trEnc.LastParsed
			} else if !trEnc.LastParsed.Empty() {
				// add the last parsed part to current header content
				h.Val.Extend(trEnc.LastParsed.EndOffs())
			}
			if err == 0 {
				h.state = hFIN
			}
			return n, err
		case hWSockProto: // continue WSockProto parsing
			wsProto := hb.GetWSProto()
			n, _, err := ParseAllWSProtoValues(buf, i, wsProto)
			// fix hdr. Val
			if h.Val.Empty() {
				h.Val = wsProto.LastParsed
			} else if !wsProto.LastParsed.Empty() {
				// add the last parsed part to current header content
				h.Val.Extend(wsProto.LastParsed.EndOffs())
			}
			if err == 0 {
				h.state = hFIN
			}
			return n, err
		case hWSockExt: // continue WSockExtensions parsing
			wsExt := hb.GetWSExt()
			n, _, err := ParseAllWSExtValues(buf, i, wsExt)
			// fix hdr. Val
			if h.Val.Empty() {
				h.Val = wsExt.LastParsed
			} else if !wsExt.LastParsed.Empty() {
				// add the last parsed part to current header content
				h.Val.Extend(wsExt.LastParsed.EndOffs())
			}
			if err == 0 {
				h.state = hFIN
			}
			return n, err
		default: // unexpected state
			return i, ErrHdrBug
		}
	}
moreBytes:
	return i, ErrHdrMoreBytes
endOfHdr:
	h.state = hFIN
	return i + crl, 0
errBadChar:
errEmptyTok:
	return i, ErrHdrBadChar
}

// ParseHeaders parses all the headers till end of header marker (double CRLF).
// It returns an offset after parsed headers and no error (0) on success.
// Special error values: ErrHdrMoreBytes - more data needed, call again
//                       with returned offset and same headers struct.
//                       ErrHdrEmpty - no headers (empty line found first)
// See also ParseHdrLine().
func ParseHeaders(buf []byte, offs int, hl *HdrLst, hb PHBodies) (int, ErrorHdr) {

	i := offs
	for i < len(buf) {
		var h *Hdr
		if hl.N < len(hl.Hdrs) {
			h = &hl.Hdrs[hl.N]
		} else {
			h = &hl.hdr
		}
		n, err := ParseHdrLine(buf, i, h, hb)
		switch err {
		case 0:
			hl.PFlags.Set(h.Type)
			hl.SetHdr(h) // save "shortcut"
			if h == &hl.hdr {
				hl.hdr.Reset() // prepare it for reuse
			}
			i = n
			hl.N++
			continue
		case ErrHdrEmpty:
			if hl.N > 0 {
				// end of headers
				return n, 0
			}
			return n, err
		case ErrHdrMoreBytes:
			fallthrough
		default:
			return n, err
		}
	}
	return i, ErrHdrMoreBytes
}
