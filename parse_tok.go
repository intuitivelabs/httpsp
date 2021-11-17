// Copyright 2021 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

package httpsp

type PToken struct {
	V PField // token value
	PTokIState
}

// internal state
type PTokIState struct {
	state  uint8 // internal state
	soffs  int   // saved internal offset
	tstart int   // token start
	tend   int   //  crt. token end
}

func (pt *PToken) Reset() {
	*pt = PToken{}
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

// internal parser states
const (
	tokInit uint8 = iota // look for token start
	tokVal               // in-token
	tokWS                // whitespace after token
	tokFNxt              // separator found, find next tok start
	tokFIN               // parsing ended
	tokERR               // pasring error
)

// parsing flags
const (
	PTokNoneF       uint = 0
	PTokCommaSepF   uint = 1 << iota // comma separated tokens
	PTokSpSepF                       // whitespace separated tokens
	PTokAllowSlashF                  // alow '/' inside the token
)

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
				n, crl, err = skipLWS(buf, i)
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
				if c <= 31 || c >= 127 {
					// no ctrl chars allowed
					ptok.state = tokERR
					return i, ErrHdrBadChar
				}
				s = i
				ptok.V.Set(i, i)
				ptok.state = tokVal
			}
		case tokVal:
			switch c {
			case ' ', '\t', '\n', '\r':
				n, crl, err = skipLWS(buf, i)
				if err == ErrHdrMoreBytes {
					// keep state and keep the offset pointing before the
					// whitespace
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
			case '(', ')', '<', '>', '@', ';', ':', '\\', '"', '[', ']',
				'?', '=', '{', '}':
				ptok.state = tokERR
				return i, ErrHdrBadChar
			case '/':
				if flags&PTokAllowSlashF == 0 {
					return i, ErrHdrBadChar
				}
				// else do nothing ('/' is part of the token)
			default:
				if c <= 31 || c >= 127 {
					// no ctrl chars allowed
					ptok.state = tokERR
					return i, ErrHdrBadChar
				}
				// keep the current state
			}
		case tokWS: // whitespace at end of token, look for start of next one
			switch c {
			case ' ', '\t', '\n', '\r':
				n, crl, err = skipLWS(buf, i)
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
				n, crl, err = skipLWS(buf, i)
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
			default:
				// 1st non-whitespace
				goto moreValues
			}
		default:
			return i, ErrHdrBug
		}
		i++
	}
moreBytes: // end of buffer reached
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
	case tokVal, tokWS, tokFNxt:
		ptok.state = tokFIN
	default:
		ptok.state = tokERR
		return n + crl, ErrHdrBug
	}
	ptok.soffs = 0
	return n + crl, retOkErr
}
