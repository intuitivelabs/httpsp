// Copyright 2021 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

package httpsp

import ()

// PHTTPMsg contains a fully or partially parsed HTTP message.
// If the message is not fully contained in the passed input, the internal
// parsing state will be saved internally and parsing can be resumed later
// when more input is available.

type PMsg struct {
	FL        PFLine   // first line request/response
	PV        PHdrVals // parsed (selected) header values
	HL        HdrLst   // headers
	Body      PField   // message body, empty if parsing body not requested
	LastChunk ChunkVal // last parsed body chunk "header" if chunked encoding
	// Data slice (copy of the original slice passed as parameter to
	// the parsing function). Parsed values will point inside it.
	// Note that the actual message starts at Buf[initial_used_offset], which
	// might be different from Buf[0]
	Buf    []byte
	RawMsg []byte // raw message data (points to parsed message: RawMsg[0:])

	// minimum space for headers containing headers broken into name: val
	// (used by default inside HL if not initialised with a bigger value)
	hdrs [10]Hdr

	PMsgIState // internal state
}

// Reset re-initializes the parsed message and the internal parsing state.
func (m *PMsg) Reset() {
	*m = PMsg{}
	m.FL.Reset()
	m.PV.Reset()
	m.HL.Reset()
	m.Body.Reset()
	m.LastChunk.Reset()
	m.PMsgIState = PMsgIState{}
}

// Init initializes a PMsg with a new message and an empty array for
// holding the parsed headers.
// If the parsed headers array is nil, the default 10-elements private
// array will be used instead (PMsg.hdrs)
func (m *PMsg) Init(msg []byte, hdrs []Hdr) {
	m.Reset()
	m.Buf = msg
	if hdrs != nil {
		m.HL.Hdrs = hdrs
	} else {
		m.HL.Hdrs = m.hdrs[:]
	}
}

// Parsed returns true if the message is fully parsed and no more
// input is needed (including the body if body parsing was requested).
func (m *PMsg) Parsed() bool {
	return m.state == MsgFIN
}

// ParsedHdrs returns true if the headers are fully parsed.
func (m *PMsg) ParsedHdrs() bool {
	return m.state == MsgFIN || m.state == MsgBodyCLen ||
		m.state == MsgBodyChunked || m.state == MsgBodyChunkedData ||
		m.state == MsgBodyEOF ||
		m.state == MsgNoBody || m.state == MsgBodyInit
}

// Err returns true if parsing failed.
func (m *PMsg) Err() bool {
	return m.state == MsgErr
}

// Request returns true if the message is a HTTP request
func (m *PMsg) Request() bool {
	return m.FL.Request()
}

// Method returns the numeric HTTP method.
// For replies it reutrn MUndef
func (m *PMsg) Method() HTTPMethod {
	if m.Request() {
		return m.FL.MethodNo
	}
	return MUndef
}

// BodyType returns the way the body is delimited.
// Parameters: prevMethod - previous request method if this is a reply
// (use MUndef if not known, but note that replies to HEAD & CONNECT need to
// have special treatment: any body length header is ignored).
// Possible return values:
//  MsgNoBody   - no body (not allowed to have body, Content-Length ignored)
//  MsgBodyCLen - fixed body size, based on Content-Length
//  MsgBodyChunked - "chunked" body, Content-Length ignored.
//  MsgBodyEOF   - body extedns till connection end / EOF
//  MsgErr - some error
// see RFC 7230 section 3.3.3.
func (m *PMsg) BodyType(prevMethod HTTPMethod) MsgPState {
	if !m.Request() { // if reply
		if (m.FL.Status > 99 && m.FL.Status < 200) ||
			m.FL.Status == 204 /* No Content */ ||
			m.FL.Status == 304 /* Not Modified */ ||
			prevMethod == MHead /* reply to a HEAD */ {
			return MsgNoBody
		}
		// 2xx response to CONNECT => tunnel => MsgBodyEOF
		// (RFC 7231 section 4.3.6)
		if prevMethod == MConnect &&
			(m.FL.Status >= 200 && m.FL.Status <= 299) {
			return MsgBodyEOF
		}
	}

	// Transfer-Encoding has priority over Content-Length
	if m.HL.PFlags&HdrTrEncodingF != 0 {
		// if Transfer-Encoding present and chunked transfer coding
		if m.PV.TrEnc.Encodings&TrEncChunkedF != 0 &&
			m.PV.TrEnc.Last.Enc == TrEncChunkedF {
			//  check if "chunked" is the final coding else
			//       fallback
			return MsgBodyChunked
		}
		if !m.Request() {
			// if Transfer-Encoding present, but chunked is not the final
			// encoding and response => body till connection end
			return MsgBodyEOF
		} else {
			// request => no reliable way to determine body end
			// theoretically the servers should close connection
			return MsgBodyEOF // TODO: or maybe better MsgErr
		}
	}

	if m.HL.PFlags&HdrCLenF != 0 {
		// TODO: if more then one Content-Length and they have different values
		// => error
		return MsgBodyCLen
	}

	if m.Request() {
		return MsgNoBody
	}
	// else reply and no body length => body ends with the connection
	return MsgBodyEOF
}

// HTTPMsgIState holds the internal parsing state
type PMsgIState struct {
	state MsgPState
	offs  int
}

type MsgPState uint8

// Parsing states
const (
	MsgInit MsgPState = iota
	MsgFLine
	MsgHeaders
	MsgBodyInit
	MsgNoBody          // message with no body
	MsgBodyCLen        // parsing body based on Content-Length
	MsgBodyChunked     // parsing body based on Transfer-Encoding chunked
	MsgBodyChunkedData // skipping over the chunk data
	MsgBodyEOF         // parsing body till connection is closed
	MsgErr
	MsgNoCLen // no Content-Length and Content-Length required
	MsgFIN    // fully parsed
)

// Parsing flags for ParseMsg()

const (
	// don't parse the body (return offset = body start)
	MsgSkipBodyF   = 1 << iota
	MsgNoMoreDataF // no more message data (e.g EOF), stop at end of buf
)

// ParseMsg parses a HTTP 1.x message contained in buf[], starting at
// offset offs. If the parsing requires more data (ErrHdrMoreBytes),
// this function should be called again with an extended buf containing the
// old data + new data and with offs equal to the last returned value
// (so that parsing will continue from that point).
// It returns the offset at which parsing finished and an error.
// On success the offset points to the first byte after the message.
// If no more input data is available (buf contains everything, e.g. EOF on
// connection) pass the MsgNoMoreDataF flag.
//  Note that a reference to buf[] will be "saved" inside msg.Buf when
// parsing is complete.
func ParseMsg(buf []byte, offs int, msg *PMsg, flags uint8) (int, ErrorHdr) {
	var err ErrorHdr
	var o = offs
	switch msg.state {
	case MsgInit:
		msg.offs = offs
		msg.state = MsgFLine
		fallthrough
	case MsgFLine:
		if o, err = ParseFLine(buf, o, &msg.FL); err != 0 {
			goto errFL
		}
		msg.state = MsgHeaders
		fallthrough
	case MsgHeaders:
		// TODO: MsgNoMoreDataF support for ParseHeaders ?
		if o, err = ParseHeaders(buf, o, &msg.HL, &msg.PV); err != 0 {
			goto errHL
		}
		msg.state = MsgBodyInit
		fallthrough
	case MsgBodyInit:
		if (flags & MsgSkipBodyF) != 0 {
			msg.Body.Set(o, o)
			goto end
		}
		fallthrough
	case MsgBodyCLen, MsgBodyEOF, MsgNoBody, MsgBodyChunked,
		MsgBodyChunkedData:
		if o, err = SkipBody(buf, o, msg, flags); err != 0 {
			goto errBody
		}
	case MsgFIN:
		// already parsed
		return o, 0
	default:
		err = ErrHdrBug
		goto errBUG
	}
end:
	// Body should be set by  SkipBody() or MsgBodyInit & MsgSkipBodyF
	msg.Buf = buf[0:o]
	msg.RawMsg = msg.Buf[msg.offs:o]
	// state when exiting should be: MsgBody*, MsgNoBody* or MsgFIN
	return o, 0
errFL:
errHL:
errBody:
errBUG:
	if err != ErrHdrMoreBytes {
		msg.state = MsgErr
	} else if (flags & MsgNoMoreDataF) != 0 {
		//msg.state = MsgErr
		err = ErrHdrTrunc
	}
	return o, err
}

// SkipBody will find the type of the message body and skip over it or
// "continue" skipping.
// It requires an initialised message with the headers parsed
// (msg.ParsedHdrs() == true).
// The parameters are: a buffer containing the message or the message
// beginning (buf), a current offset in the buffer (returned by a previous
// SkipBody() or ParseMsg() call), a HTTP parsed message structure with
// the header parsed (msg) and some parsing flags
// ( MsgSkipBodyF and MsgNoMoreDataF).
//
// It is used internally by ParseMsg if MsgSkipBodyF is not specified.
//
// It returns a new offset and an error. If the error is ErrHdrMoreBytes it
// means that the passed buffer does not contain the whole body and this
// function should be called again with the returned offset and an extended
// buffer (with the original content + additional bytes).
// On success the offset points to the first byte after the whole message.
func SkipBody(buf []byte, offs int, msg *PMsg, flags uint8) (int, ErrorHdr) {
	var o = offs
retry:
	switch msg.state {
	case MsgBodyInit:
		msg.Body.Set(o, o)
		msg.state = msg.BodyType(MUndef) // TODO: pass prev req, meth.
		if msg.state == MsgErr {
			return o, ErrHdrNoCLen // TODO: better error ?
		}
		if msg.state == MsgBodyInit {
			goto errBUG
		}
		goto retry
	case MsgNoBody:
		msg.Body.Set(0, 0)
		// do nothing, end
	case MsgBodyCLen:
		if (flags & MsgSkipBodyF) != 0 {
			goto end
		}
		if msg.PV.CLen.Parsed() {
			// skip msg.PV.CLen.Len bytes
			if (o + int(msg.PV.CLen.UIVal)) > len(buf) {
				if !msg.Body.OffsIn(o) {
					msg.Body.Extend(o)
				}
				if (flags & MsgNoMoreDataF) != 0 {
					// allow truncated body (?)
					o = len(buf)
					goto end
				}
				// keep start-of-body offset (we use it on success/full body)
				return o, ErrHdrMoreBytes
			}
			o += int(msg.PV.CLen.UIVal)
		} else {
			// no CLen parsed but CLen based state -> BUG
			goto errBUG
		}
	case MsgBodyEOF:
		if (flags & MsgNoMoreDataF) != 0 {
			// use whole buf
			o = len(buf)
		} else {
			// eat everything till connection end
			if !msg.Body.OffsIn(o) {
				msg.Body.Extend(o)
			}
			return o, ErrHdrMoreBytes
		}
	case MsgBodyChunked:
		if (flags & MsgSkipBodyF) != 0 {
			goto end
		}
		var err ErrorHdr
		o, _, err = ParseChunk(buf, o, &msg.LastChunk)
		if err == 0 {
			// skip over chunk body
			msg.state = MsgBodyChunkedData
			goto retry
		} else if err == ErrHdrMoreBytes {
			return o, err // retry ParseChunk when more bytes are available
		}
		// else other error => fail
		return o, err
	case MsgBodyChunkedData:
		if (flags & MsgSkipBodyF) != 0 {
			goto end
		}
		nxt := o + int(msg.LastChunk.Size) + 2 /* CRLF */
		// skip current chunk bytes + delimiting CRLF
		if nxt > len(buf) {
			if !msg.Body.OffsIn(o) {
				msg.Body.Extend(o)
			}
			if (flags & MsgNoMoreDataF) != 0 {
				// allow truncated body (?)
				o = len(buf)
				goto end
			}
			// keep start-of-body offset (we use it on success/full body)
			return o, ErrHdrMoreBytes
		}
		o = nxt
		if msg.LastChunk.Size == 0 {
			// last chunk (empty) => stop
			goto end
		}
		// current chunk fully parsed, switch back to parsing chunk headers
		msg.LastChunk.Reset()
		msg.state = MsgBodyChunked
		goto retry
	default:
		// unknown state
		goto errBUG
	}
	msg.Body.Extend(o)
end:
	msg.Buf = buf[0:o]
	msg.RawMsg = msg.Buf[msg.offs:o]
	msg.state = MsgFIN
	return o, 0
errBUG:
	return o, ErrHdrBug
}
