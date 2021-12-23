// Copyright 2021 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a BSD-style license
// that can be found in the LICENSE_BSD.txt file in the root of the source
// tree.

package httpsp

import (
	"fmt"
)

// PToken contains a parsed token, complete with internal parsing state
// (that would allow continuing parsing in some cases).
// Generic token format:  token ["/" sub-name] *(";" param "=" val)
type PToken struct {
	V         PField      // complete token (name/suffix)
	SepOffs   OffsT       // sep offset (between name & suffix) or 0
	Params    PField      // complete params string
	ParamsNo  uint        // number of parameters
	LastParam PTokParam   // last token parameter, parsed
	ParamLst  []PTokParam // slice to be filled with parsed params
	PTokIState
}

// internal state
type PTokIState struct {
	state uint8 // internal state
	soffs int   // saved internal offset
}

func (pt *PToken) Reset() {
	paramLst := pt.ParamLst
	*pt = PToken{}
	pt.ParamLst = paramLst // save param list placeholder
}

func (pt *PToken) Empty() bool {
	return pt.state == tokInit
}

func (pt *PToken) Parsed() bool {
	return pt.state == tokFIN || pt.state == tokWS || pt.state == tokFNxt
}

func (pt *PToken) Finished() bool {
	return pt.state == tokFIN || pt.state == tokERR
}

func (pt *PToken) Pending() bool {
	return pt.state != tokFIN && pt.state != tokInit && pt.state != tokERR
}

// Name returns only the name part of the token.
// E.g.: for HTTP/1.1 it would return only "HTTP".
func (pt *PToken) Name() PField {
	if pt.SepOffs != 0 {
		var n PField
		n.Set(int(pt.V.Offs), int(pt.SepOffs))
		return n
	}
	return pt.V
}

// Suffix returns the suffix or the sub-name part of the token.
// E.g.: for HTTP/1.1 it would return only "1.1" and for sip  "".
func (pt *PToken) Suffix() PField {
	if pt.SepOffs != 0 {
		s := pt.V
		s.Offs = pt.SepOffs + 1
		return s
	}
	return PField{0, 0}
}

// Param returns the n-th parameter (starting from 0).
// If no parameter found (too few) returns an empty PTokParam & ErrHdrEmpty.
// On parse error returns the corresponding ErrorHdr and an empty PTokParam.
func (pt *PToken) Param(buf []byte, n int, flags uint) (PTokParam, ErrorHdr) {
	if uint(n) >= pt.ParamsNo || n < 0 || pt.Params.Empty() {
		return PTokParam{}, ErrHdrEmpty
	}
	if n < len(pt.ParamLst) {
		// if saved in ParmLst => return directly
		return pt.ParamLst[n], ErrHdrOk
	}
	o := int(pt.Params.Offs)
	buf = buf[:o+int(pt.Params.Len)]
	var err ErrorHdr
	var param PTokParam
	i := 0
	for {
		param.Reset()
		o, err = ParseTokenParam(buf, o, &param, flags|PTokInputEndF)
		if err != ErrHdrOk && err != ErrHdrEmpty && err != ErrHdrMoreValues &&
			err != ErrHdrEOH {
			return PTokParam{}, err
		}
		if i == n {
			if err == ErrHdrOk || err == ErrHdrMoreValues {
				return param, ErrHdrOk
			}
			if err == ErrHdrEOH {
				if param.All.Len == 0 {
					return param, ErrHdrEmpty
				}
				return param, ErrHdrOk
			}
			return PTokParam{}, err
		}
		if err != ErrHdrMoreValues {
			if err == ErrHdrOk || err == ErrHdrEOH {
				// no more values => return HdrEmpty
				return PTokParam{}, ErrHdrEmpty
			}
			return PTokParam{}, err
		}
		i++
	}
	return PTokParam{}, ErrHdrBug
}

// PTokParam contains a token parameter.
// E.g. foo;p1=v1 => PTokParam will contain the parsed "p1=5" part.
type PTokParam struct {
	All   PField // complete parameter field ( name = value), e.g.: "p1=v1"
	Name  PField // param. name with stripped whitespace (e.g. "p1")
	Val   PField // param value with stripped whitespace (e.g. "v1")
	state uint8  // internal state
}

func (pt *PTokParam) Reset() {
	*pt = PTokParam{}
}

func (pt *PTokParam) Empty() bool {
	return pt.All.Empty()
}

// internal parser states
const (
	tokInit     uint8 = iota // look for token start
	tokName                  // in-token
	tokWS                    // whitespace after token
	tokFNxt                  // separator found, find next tok start
	tokFParam                // ';' found, look for parameter start
	tokPName                 // inside parameter name
	tokPVal                  // parsing parameters value
	tokPVQuoted              // inside quoted param vale
	tokFIN                   // parsing ended
	tokERR                   // pasring error
)

// parsing flags
const (
	PTokNoneF        uint = 0
	PTokCommaSepF    uint = 1 << iota // comma separated tokens
	PTokSpSepF                        // whitespace separated tokens
	PTokAllowSlashF                   // alow '/' inside the token
	PTokAllowParamsF                  // alow ;param=val
	PTokInputEndF                     // inputs end at end of buf
)

// returns true if c is an allowed ascii char inside a token name or value
func tokAllowedChar(c byte) bool {
	if c <= 32 || c >= 127 {
		// no ctrl chars,  non visible chars or white space allowed
		// (see rfc7230 3.2.6)
		return false
	}
	return true
}

// ParseTokenLst iterates through a comma or space separated token list,
// returning each token in turn. The "flags" parameter controls whether
// it is supposed to parse a comma separated token list (PTokCommaSepF),
// white space  separated (PTokSpSepF), a mix of both
// (PTokSpSepF|PTokCommaSepF) or a single token.
// Any number of separators are accepted between tokens. White space is
// always trimmed from the result.
// The parameters are:  a message buffer and an offset inside from which
// the parsing should start or continue, a pointer to a token structure that
// will be filled on success and some parsing flags.
// It returns a new offset pointing on success immediately after the end of
// the header (it expects a line to be terminated by CR LF followed by
// a non white-space character) or to the start of the next token if more
// token are present (in this case the error reported will ErrHdrMoreValues).
// If the end of the token list cannot be found, it will return
// ErrHdrMoreBytes and offset for resuming parsing when more data was added
// to the buffer (the function should be called again after apending more
// bytes to the buffer. with the last returned offset and the same "untouched"
// PToken structure).
// If ErrHdrMoreValues is returned it means there are more token to be parsed
// and after using the current token in ptok, the function can be called
// again with a fresh ptok (or a reset-ed ptok) and the returned offset to
// continue parsing the next token.
// Any other ErrHdr* besider ErrHdrOK and the 2 above values, means parsing
// has failed and the returned offset will point to the character that
// triggered the error.
func ParseTokenLst(buf []byte, offs int, ptok *PToken, flags uint) (int, ErrorHdr) {

	if ptok.state == tokFIN {
		// called again after finishing
		return offs, 0
	}
	var s = ptok.soffs
	i := offs
	var n, crl int // next non lws and crlf length
	var err, retOkErr ErrorHdr

	for i < len(buf) {
		c := buf[i]
		switch ptok.state {
		case tokInit: // -> search for token start
			switch c {
			case ' ', '\t', '\n', '\r':
				n, crl, err = skipLWS(buf, i, flags)
				if err == 0 {
					i = n
					continue
				}
				if err == ErrHdrEOH {
					// end of header
					goto endOfHdr
				}
				if err == ErrHdrMoreBytes {
					i = n
					goto moreBytes
				}
				ptok.state = tokERR
				return n, err
			case ',':
				if flags&PTokCommaSepF != 0 {
					i++ // skip over extra ','
					continue
				} else {
					ptok.state = tokERR
					return i, ErrHdrBadChar
				}
			case '(', ')', '<', '>', '@', ';', ':', '\\', '"', '[', ']',
				'?', '=', '{', '}', '/':
				ptok.state = tokERR
				return i, ErrHdrBadChar
			default:
				if !tokAllowedChar(c) {
					// no ctrl chars allowed
					ptok.state = tokERR
					return i, ErrHdrBadChar
				}
				s = i
				ptok.V.Set(i, i)
				ptok.state = tokName
			}
		case tokName:
			switch c {
			case ' ', '\t', '\n', '\r':
				n, crl, err = skipLWS(buf, i, flags)
				if err == ErrHdrMoreBytes {
					// keep state and keep the offset pointing before the
					// whitespace
					// TODO: n == cr lf start & crl == 0  && end of input flag?
					goto moreBytes
				}
				ptok.state = tokWS
				ptok.V.Extend(i)
				if err == 0 {
					i = n
					continue
				}
				if err == ErrHdrEOH {
					goto endOfHdr
				}
				ptok.state = tokERR
				return n, err
			case ',':
				if flags&PTokCommaSepF != 0 {
					ptok.V.Extend(i)
					ptok.state = tokFNxt
				} else {
					ptok.state = tokERR
					return i, ErrHdrBadChar
				}
			case '(', ')', '<', '>', '@', ':', '\\', '"', '[', ']',
				'?', '=', '{', '}':
				ptok.state = tokERR
				return i, ErrHdrBadChar
			case '/':
				if flags&PTokAllowSlashF == 0 {
					return i, ErrHdrBadChar
				}
				ptok.SepOffs = OffsT(i) // save '/' position
			case ';':
				if flags&PTokAllowParamsF == 0 {
					return i, ErrHdrBadChar
				}
				ptok.V.Extend(i)
				ptok.state = tokFParam
			default:
				if !tokAllowedChar(c) {
					// no ctrl chars allowed
					ptok.state = tokERR
					return i, ErrHdrBadChar
				}
				// keep the current state
			}
		case tokWS: // whitespace at end of token, look for start of next one
			switch c {
			case ' ', '\t', '\n', '\r':
				n, crl, err = skipLWS(buf, i, flags)
				if err == ErrHdrMoreBytes {
					// keep state and keep the offset pointing before the
					// whitespace
					goto moreBytes
				}
				if err == 0 {
					i = n
					continue
				}
				if err == ErrHdrEOH {
					goto endOfHdr
				}
				ptok.state = tokERR
				return n, err
			case ',':
				if flags&PTokCommaSepF != 0 {
					ptok.state = tokFNxt
				} else {
					ptok.state = tokERR
					return i, ErrHdrBadChar
				}
			case ';':
				if flags&PTokAllowParamsF == 0 {
					return i, ErrHdrBadChar
				}
				ptok.state = tokFParam
			default:
				// 1st non-whitespace
				if flags&PTokSpSepF != 0 {
					// allow whitespace separated tokens
					// => new token start found
					goto moreValues
				}
				//  else new token without a separating ','
				ptok.state = tokERR
				return i, ErrHdrBadChar
			}
		case tokFNxt: // token sep found, look for start of next one
			switch c {
			case ' ', '\t', '\n', '\r':
				n, crl, err = skipLWS(buf, i, flags)
				if err == ErrHdrMoreBytes {
					// keep state and keep the offset pointing before the
					// whitespace
					goto moreBytes
				}
				if err == 0 {
					i = n
					continue
				}
				if err == ErrHdrEOH {
					goto endOfHdr
				}
				ptok.state = tokERR
				return n, err
			case ',':
				if flags&PTokCommaSepF == 0 {
					ptok.state = tokERR
					return i, ErrHdrBadChar
				}
				// else do nothing (ignore multiple ',')
			case '(', ')', '<', '>', '@', ';', ':', '\\', '"', '[', ']',
				'?', '=', '{', '}', '/':
				// token starts with un-allowed char
				ptok.state = tokERR
				return i, ErrHdrBadChar
			default:
				// 1st non-whitespace
				goto moreValues
			}
		case tokFParam: //';' found, look for param start
			// FIXME? ptok.LastParam.Reset()
			n, err = ParseTokenParam(buf, i, &ptok.LastParam, flags)
			//fmt.Printf("DBG: ParseTokenParam %q  i = %d [%q] n = %d err = %q state %d LastParam %v\n", buf, i, buf[i:], n, err, ptok.state, ptok.LastParam)
			// change state only if token separator found
			if err == ErrHdrMoreBytes {
				// keep the state
				i = n
				goto moreBytes
			}
			if !ptok.LastParam.All.Empty() {
				if len(ptok.ParamLst) > int(ptok.ParamsNo) {
					ptok.ParamLst[ptok.ParamsNo] = ptok.LastParam
					fmt.Printf("DBG: ParseTokenParam %q  setting ParamLst[%d] to %q\n", buf, ptok.ParamsNo, ptok.LastParam.All.Get(buf))
				}
				ptok.ParamsNo++
				if ptok.Params.Empty() {
					ptok.Params = ptok.LastParam.All
				} else {
					ptok.Params.Extend(int(ptok.LastParam.All.Offs) +
						int(ptok.LastParam.All.Len))
				}
			}
			if err == ErrHdrMoreValues {
				// keep the state and parse more params
				i = n
				continue
			}
			if err == 0 {
				// end of params found
				ptok.state = tokFNxt
				i = n + 1 // n = sep position
				if n >= len(buf) {
					goto moreBytes
				}
				continue
			}
			if err == ErrHdrEOH {
				goto endOfHdr
			} else {
				ptok.state = tokERR
				return n, err
			}
		default:
			return i, ErrHdrBug
		}
		i++
	}
	//fmt.Printf("DBG: end of input for %q, i = %d n = %d crl = %d state %d\n", buf, i, n, crl, ptok.state)
moreBytes: // end of buffer reached
	//fmt.Printf("DBG: moreBytes %q, i = %d n = %d crl = %d state %d\n", buf, i, n, crl, ptok.state)
	// end of buffer, but couldn't find end of headers
	// i == len(buf) or
	// i = first space before the end & n == ( len(buf) or  position of
	//  last \n or \r before end of buf -- len(buf) -1)
	if flags&PTokInputEndF != 0 { // end of input - force end of headers
		switch ptok.state {
		case tokInit, tokWS, tokFNxt, tokFParam:
			// do nothing (end of input in init stat or eating whitespace state
			// handled in endOfHdr
		case tokName:
			// save name end
			ptok.V.Extend(i)
		default:
			ptok.state = tokERR
			return i, ErrHdrBug
		}
		crl = 0
		ptok.soffs = s      // ? could be skipped since parsing ends ?
		n = len(buf)        // report the whole buf as "parsed" (or n = i?)
		retOkErr = ErrHdrOk // or ErrHdrMoreBytes ?
		goto endOfHdr
	}
	ptok.soffs = s
	return i, ErrHdrMoreBytes
moreValues: // end of current token but more present (',')
	retOkErr = ErrHdrMoreValues
	n = i
	crl = 0
	switch ptok.state {
	case tokWS, tokFNxt:
		// do nothing (keep state)
	default:
		ptok.state = tokERR
		return n + crl, ErrHdrBug
	}
	ptok.soffs = 0
	return n + crl, retOkErr
endOfHdr: // end of header found
	// here i will point to first WS char (including CR & LF)
	//      n will point to the line end (CR or LF)
	//      crl will contain the line end length (1 or 2) so that
	//      n+crl is the first char in the new header
	switch ptok.state {
	case tokInit:
		// end of header without finding a token = > bad token
		return n + crl, ErrHdrEmpty
	case tokName, tokWS, tokFNxt, tokFParam:
		ptok.state = tokFIN
	default:
		ptok.state = tokERR
		return n + crl, ErrHdrBug
	}
	ptok.soffs = 0
	return n + crl, retOkErr
}

// SkipQuoted skips a quoted string, looking for the end quote.
// It handles escapes. It escapes to be called with an offset pointing
// _inside_ some open quotes (after the '"' character).
// On success it returns and offset after the closing quote.
// If there are not enough bytes to find the end, it will return
// ErrHdrMoreBytes and an offset (which can be used to continue parsing after
// more bytes have been added to buf).
// It doesn't allow CR or LF inside the quoted string (see rfc7230 3.2.6).
func SkipQuoted(buf []byte, offs int) (int, ErrorHdr) {
	i := offs
	// var n, crl int // next non lws and crlf length
	// var err, retOkErr ErrorHdr

	for i < len(buf) {
		c := buf[i]
		switch c {
		case '"':
			return i + 1, ErrHdrOk
		case '\\': // quoted-pair
			if (i + 1) < len(buf) {
				if buf[i+1] == '\r' || buf[i+1] == '\n' {
					// CR or LF not allowed in escape pairs
					return i + 1, ErrHdrBadChar
				}
				i += 2 // skip '\x'
				continue
			}
			goto moreBytes

			// -- don't allow \n or \r in quotes (see rfc 7230 3.2.6)
		case '\n', '\r', 0x7f:
			return i, ErrHdrBadChar
		default:
			if c < 0x21 && c != ' ' && c != '\t' {
				return i, ErrHdrBadChar
			}
			/*
				case ' ', '\t', '\n', '\r':
					n, crl, err = skipLWS(buf, i, flags)
					if err == 0 {
						i = n
						continue
					}
					if err == ErrHdrEOH {
						goto endOfHdr
					}
					if err == ErrHdrMoreBytes {
						i = n
						goto moreBytes
					}
					return n, err
			*/
		}
		i++
	}
moreBytes:
	return i, ErrHdrMoreBytes
	/*
		endOfHdr: // end of header found
			// here i will point to first WS char (including CR & LF)
			//      n will point to the line end (CR or LF)
			//      crl will contain the line end length (1 or 2) so that
			//      n+crl is the first char in the new header
			// unexpected end inside quotes !
			return n + crl, ErrHdrBad
	*/
}

// ParseTokenParam will parse a string of the form param [= value] [;] .
// param has to be a valid token. value can be a token or a quoted string.
// White space is allowed before and after "=".
// The value part might be missing (e.g. ";lr").
// The string is also terminated by a token list separator
// (either ',' , whitespace after value and with no ';' or both, depending
// on the flags), in which case the returned offset will be the separator
// offset.
// If there are more parameters present (';' separated), the returned offset
// will be the start of the next parameter (after ';' and possible whitespace)
// and the returned error will be ErrHdrMoreValues.
// Return values:
//  - offs, ErrHdrOK - parsed full param and this is the last parameter. offs is // the offset of the next token separator or past the end of the buffer.
//  - offs. ErrHdrEOH - parsed full param and encountered end of header
//  (\r\nX). offs points at the first char after the end of header or past
//   the end of buffer.
// - offs, ErrHdrMoreValues - parsed full param and there are more values.
// offs is the start offset of the next parameter (leading white space trimmed)
// - offs, ErrHdrEmpty - empty parameter
// - offs. ErrHdrMoreBytes - more bytes are needed to finish parsing. offs
//  can be used to resume parsing (along with the same param).
// - any other ErrorHdr value -> parsing error and the offset of the 1st
// offending char.
func ParseTokenParam(buf []byte, offs int, param *PTokParam, flags uint) (int, ErrorHdr) {

	// internal state
	const (
		paramInit uint8 = iota
		paramName
		paramFEq
		paramFVal
		paramVal
		paramFSemi
		paramFNxt
		paramInitNxtVal // more for nice debugging
		paramQuotedVal
		paramERR
		paramFIN
	)

	if param.state == paramFIN {
		// called again after finishing
		return offs, 0
	}
	i := offs
	var n, crl int // next non lws and crlf length
	var err, retOkErr ErrorHdr

	for i < len(buf) {
		c := buf[i]
		n = 0
		switch param.state {
		case paramInit, paramInitNxtVal, paramFNxt:
			switch c {
			case ' ', '\t', '\n', '\r':
				n, crl, err = skipLWS(buf, i, flags)
				if err == ErrHdrMoreBytes {
					// keep state and keep the offset pointing before the
					// whitespace
					goto moreBytes
				}
				// keep state
				if err == 0 {
					i = n
					continue
				}
				if err == ErrHdrEOH {
					goto endOfHdr
				}
				return n, err
			case ';':
				// do nothing, allow empty params, just skip them
			case '(', ')', '<', '>', '@', ':', '\\', '"', '[', ']',
				'?', '=', '{', '}', '/', ',':
				// param name starts with un-allowed char
				param.state = paramERR
				return i, ErrHdrBadChar
			default:
				if !tokAllowedChar(c) {
					param.state = paramERR
					return i, ErrHdrBadChar
				}
				if param.state == paramFNxt {
					goto moreValues
				}
				param.state = paramName
				param.Name.Set(i, i)
				param.All.Set(i, i)
			}
		case paramName:
			switch c {
			case ' ', '\t', '\n', '\r':
				n, crl, err = skipLWS(buf, i, flags)
				if err == ErrHdrMoreBytes {
					// keep state and keep the offset pointing before the
					// whitespace
					goto moreBytes
				}
				param.state = paramFEq
				param.Name.Extend(i)
				param.All.Extend(i)
				if err == 0 {
					i = n
					continue
				}
				if err == ErrHdrEOH {
					goto endOfHdr
				}
				return n, err
			case ';':
				// param with no value found, allow
				param.Name.Extend(i)
				param.All.Extend(i)
				param.state = paramFNxt
			case '=':
				param.Name.Extend(i)
				param.All.Extend(i + 1)
				param.state = paramFVal
			case ',':
				if flags&PTokCommaSepF != 0 {
					param.Name.Extend(i)
					param.All.Extend(i)
					param.state = paramFIN
					return i, ErrHdrOk
				}
				// param name contains un-allowed char
				param.state = paramERR
				return i, ErrHdrBadChar
			case '(', ')', '<', '>', '@', ':', '\\', '"', '[', ']',
				'?', '{', '}', '/':
				// param name contains un-allowed char
				param.state = paramERR
				return i, ErrHdrBadChar
			default:
				if !tokAllowedChar(c) {
					param.state = paramERR
					return i, ErrHdrBadChar
				}
				// do nothing
			}
		case paramFEq: // look for '=' | ';' |','
			switch c {
			case ' ', '\t', '\n', '\r':
				n, crl, err = skipLWS(buf, i, flags)
				if err == ErrHdrMoreBytes {
					// keep state and keep the offset pointing before the
					// whitespace
					goto moreBytes
				}
				// keep state
				if err == 0 {
					i = n
					continue
				}
				if err == ErrHdrEOH {
					goto endOfHdr
				}
				return n, err
			case ';':
				// param with no value found, allow
				param.state = paramFNxt
			case '=':
				param.state = paramFVal
			case ',':
				if flags&PTokCommaSepF != 0 {
					param.state = paramFIN
					return i, ErrHdrOk
				}
				// param name contains un-allowed char
				param.state = paramERR
				return i, ErrHdrBadChar
			case '(', ')', '<', '>', '@', ':', '\\', '"', '[', ']',
				'?', '{', '}', '/':
				// param name starts with un-allowed char
				param.state = paramERR
				return i, ErrHdrBadChar
			default:
				if !tokAllowedChar(c) {
					param.state = paramERR
					return i, ErrHdrBadChar
				}
				if flags&PTokSpSepF != 0 {
					// found new space separated token after param name
					// e.g.: foo;p1 bar => consider bar new param
					param.state = paramFIN
					// return separator pos (as expected)
					if i >= offs+1 {
						return i - 1, ErrHdrOk
					} else {
						return i, ErrHdrOk
					}
				}
				// looking for '=' or ';', but found another token => error
				param.state = paramERR
				return i, ErrHdrBadChar
			}
		case paramFVal:
			switch c {
			case ' ', '\t', '\n', '\r':
				n, crl, err = skipLWS(buf, i, flags)
				if err == ErrHdrMoreBytes {
					// keep state and keep the offset pointing before the
					// whitespace
					goto moreBytes
				}
				// keep state
				if err == 0 {
					i = n
					continue
				}
				if err == ErrHdrEOH {
					goto endOfHdr
				}
				return n, err
			case ';':
				// empty val (allow)
				param.Val.Set(i, i)
				param.All.Extend(i)
				param.state = paramFNxt
			case ',':
				if flags&PTokCommaSepF != 0 {
					// empty val (allow)
					param.Val.Set(i, i)
					param.state = paramFIN
					return i, ErrHdrOk
				}
				// param name contains un-allowed char
				param.state = paramERR
				return i, ErrHdrBadChar
			case '"':
				param.Val.Set(i, i)
				param.All.Extend(i)
				param.state = paramQuotedVal
			case '(', ')', '<', '>', '@', ':', '\\', '[', ']',
				'?', '=', '{', '}', '/':
				// param value starts with un-allowed char
				param.state = paramERR
				return i, ErrHdrBadChar
			default:
				if !tokAllowedChar(c) {
					param.state = paramERR
					return i, ErrHdrBadChar
				}
				param.state = paramVal
				param.Val.Set(i, i)
				param.All.Extend(i)
			}
		case paramVal:
			switch c {
			case ' ', '\t', '\n', '\r':
				n, crl, err = skipLWS(buf, i, flags)
				if err == ErrHdrMoreBytes {
					// keep state and keep the offset pointing before the
					// whitespace
					goto moreBytes
				}
				param.state = paramFSemi
				param.Val.Extend(i)
				param.All.Extend(i)
				if err == 0 {
					i = n
					continue
				}
				if err == ErrHdrEOH {
					goto endOfHdr
				}
				return n, err
			case ';':
				// empty val (allow)
				param.Val.Extend(i)
				param.All.Extend(i)
				param.state = paramFNxt
			case ',':
				if flags&PTokCommaSepF != 0 {
					// empty val (allow)
					param.Val.Extend(i)
					param.All.Extend(i)
					param.state = paramFIN
					return i, ErrHdrOk
				}
				// param name contains un-allowed char
				param.state = paramERR
				return i, ErrHdrBadChar
			case '(', ')', '<', '>', '@', ':', '\\', '"', '[', ']',
				'?', '=', '{', '}', '/':
				// param value contains un-allowed char
				param.state = paramERR
				return i, ErrHdrBadChar
			default:
				if !tokAllowedChar(c) {
					param.state = paramERR
					return i, ErrHdrBadChar
				}
			}
		case paramQuotedVal:
			n, err = SkipQuoted(buf, i)
			if err == ErrHdrMoreBytes {
				// keep state
				i = n
				goto moreBytes
			}
			if err == 0 {
				i = n
				param.Val.Extend(i)
				param.All.Extend(i)
				param.state = paramFSemi
				continue
			}
			if err == ErrHdrEOH {
				goto endOfHdr
			}
			return n, err
		case paramFSemi: // look for ';' | ',' |' ' tok   after param value
			switch c {
			case ' ', '\t', '\n', '\r':
				n, crl, err = skipLWS(buf, i, flags)
				if err == ErrHdrMoreBytes {
					// keep state and keep the offset pointing before the
					// whitespace
					goto moreBytes
				}
				// keep state
				if err == 0 {
					i = n
					continue
				}
				if err == ErrHdrEOH {
					goto endOfHdr
				}
				return n, err
			case ';':
				param.state = paramFNxt
			case ',':
				if flags&PTokCommaSepF != 0 {
					param.state = paramFIN
					return i, ErrHdrOk
				}
				// unexpected ',' after param value (if ',' not allowed as sep)
				param.state = paramERR
				return i, ErrHdrBadChar
			case '(', ')', '<', '>', '@', ':', '\\', '"', '[', ']',
				'?', '{', '}', '/', '=':
				// param name starts with un-allowed char
				param.state = paramERR
				return i, ErrHdrBadChar
			default:
				if !tokAllowedChar(c) {
					param.state = paramERR
					return i, ErrHdrBadChar
				}
				if flags&PTokSpSepF != 0 {
					// found new space separated token after param value
					// e.g.: foo;p1=5 bar =>  consider bar new param
					param.state = paramFIN
					// return separator pos (as expected)
					if i >= offs+1 {
						return i - 1, ErrHdrOk
					} else {
						return i, ErrHdrOk
					}
				}
				// looking for '=' or ';', but found another token => error
				param.state = paramERR
				return i, ErrHdrBadChar
			}
		}
		i++
	}
moreBytes:
	// end of buffer, but couldn't find end of headers
	// i == len(buf) or
	// i = first space before the end & n == ( len(buf) or  position of
	//  last \n or \r before end of buf -- len(buf) -1)
	if flags&PTokInputEndF != 0 { // end of input - force end of headers
		switch param.state {
		case paramInit, paramInitNxtVal, paramFNxt, paramFSemi,
			paramFVal, paramFEq:
			// do nothing
		case paramName:
			// end while parsing param name => param w/o value
			param.Name.Extend(i)
			param.All.Extend(i)
		case paramVal:
			param.Val.Extend(i)
			param.All.Extend(i)
		case paramQuotedVal:
			// error, open quote
			return i, ErrHdrMoreBytes
		default:
			return i, ErrHdrBug
		}
		crl = 0
		n = len(buf)        // report the whole buf as "parsed" (or n = i?)
		retOkErr = ErrHdrOk // or ErrHdrMoreBytes ?
		goto endOfHdr
	}
	return i, ErrHdrMoreBytes
moreValues:
	// here i will point to the first char of the new value
	retOkErr = ErrHdrMoreValues
	n = i
	crl = 0
	switch param.state {
	case paramFNxt:
		// init state but for next param
		param.state = paramInitNxtVal
	default:
		param.state = paramERR
		return n + crl, ErrHdrBug
	}
	return n + crl, retOkErr
endOfHdr: // end of header found
	// here i will point to first WS char (including CR & LF)
	//      n will point to the line end (CR or LF)
	//      crl will contain the line end length (1 or 2) so that
	//      n+crl is the first char in the new header
	switch param.state {
	case paramInit, paramInitNxtVal:
		// end of header without finding a param = > empty
		return n + crl, ErrHdrEOH
	case paramFNxt, paramName, paramFEq, paramFVal, paramVal, paramFSemi:
		param.state = paramFIN
	default:
		param.state = paramERR
		return n + crl, ErrHdrBug
	}
	return n + crl, ErrHdrEOH
}
