// Copyright 2022 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

package httpsp

import (
	"math/rand"
	"testing"
)

func TestSkipBodyChunk(t *testing.T) {
	// reuse chunkTests from parse_chunk_test.go

	for _, c := range chunkTests {
		chunkHdr := unescapeCRLF(c.chHdr)
		chunkD := unescapeCRLF(c.chData)
		buf := make([]byte, len(chunkHdr)+len(chunkD))
		copy(buf, chunkHdr)
		copy(buf[len(chunkHdr):], chunkD)
		if c.e.offs <= 0 { // data offset
			// fill expected offset: offs(first \n)+1
			// (works only if no last chunk with trailer headers)
			c.e.offs = len(chunkHdr)
		}
		if c.e.chkSz < 0 {
			// fill expected chunk size: data size - 2 ("\r\n")
			c.e.chkSz = int64(len(chunkD) - 2)
		}
		testSkipBodyChunk(t, buf, &c)
	}
}

func TestSkipBodyChunkPieces(t *testing.T) {
	// reuse chunkTests from parse_chunk_test.go

	for _, c := range chunkTests {
		chunkHdr := unescapeCRLF(c.chHdr)
		chunkD := unescapeCRLF(c.chData)
		buf := make([]byte, len(chunkHdr)+len(chunkD))
		copy(buf, chunkHdr)
		copy(buf[len(chunkHdr):], chunkD)
		if c.e.offs <= 0 { // data offset
			// fill expected offset: offs(first \n)+1
			// (works only if no last chunk with trailer headers)
			c.e.offs = len(chunkHdr)
		}
		if c.e.chkSz < 0 {
			// fill expected chunk size: data size - 2 ("\r\n")
			c.e.chkSz = int64(len(chunkD) - 2)
		}
		testSkipBodyChunkPieces(t, buf, &c, 10)
	}
}

func TestSkipBodyChunks(t *testing.T) {
	// reuse chunkTests from parse_chunk_test.go
	const tests = 100
	buf := make([]byte, 2048)

	for k := 0; k < tests; k++ {
		retry := 0
		nChks := 0
		buf = buf[0:rand.Intn(100)]
		offs := len(buf)
		for {
			i := rand.Intn(len(chunkTests))
			c := chunkTests[i]
			// fix testcase
			chunkHdr := unescapeCRLF(c.chHdr)
			chunkD := unescapeCRLF(c.chData)
			if c.e.offs <= 0 { // data offset
				// fill expected offset: offs(first \n)+1
				// (works only if no last chunk with trailer headers)
				c.e.offs = len(chunkHdr)
			}
			if c.e.chkSz < 0 {
				// fill expected chunk size: data size - 2 ("\r\n")
				c.e.chkSz = int64(len(chunkD) - 2)
			}

			if c.e.chkSz == 0 && nChks < 1 && retry < 1000 {
				retry++
				// this is a terminating chunk, skip over it
				continue
			}

			// add test chunk to our "msg body"
			buf = append(buf, chunkHdr...)
			buf = append(buf, chunkD...)
			nChks++
			if c.e.chkSz == 0 {
				// terminating chunk added => run test & exit
				testSkipBodyChunkedBlock(t, buf, offs)
				testSkipBodyChunkedBlockPieces(t, buf, offs, 10)
				break
			}
			if nChks > 100 {
				t.Errorf("too many chunks %d, buf len %d\n", nChks, len(buf))
				break
			}
		}
	}
}

type pMsgExpR struct {
	err    ErrorHdr
	offs   int
	nHdrs  int
	hdrf   HdrFlags
	status uint16
	m      HTTPMethod
	state  MsgPState
}

type pMsgTestCase struct {
	hdrs string // header part
	body string // msg body
	flgs uint8  // parse flags

	desc string // test description
	e    pMsgExpR
}

var msgTests = [...]pMsgTestCase{
	{
		hdrs: `HTTP/1.1 200 OK\r
Date: Sun, 10 Oct 2010 23:26:07 GMT\r
Server: Apache/2.2.8 (Ubuntu) mod_ssl/2.2.8 OpenSSL/0.9.8g\r
Last-Modified: Sun, 26 Sep 2010 22:04:35 GMT\r
ETag: "45b6-834-49130cc1182c0"\r
Accept-Ranges: bytes\r
Content-Length: 12\r
Connection: close\r
Content-Type: text/html\r
`,
		body: `Hello world!`,
		flgs: 0,
		desc: "200 with body & content-length",
		e: pMsgExpR{
			err:    0,
			offs:   0, // auto-fill
			nHdrs:  8,
			hdrf:   HdrServerF | HdrCLenF | HdrConnectionF | HdrOtherF,
			status: 200, m: 0,
			state: MsgFIN,
		},
	},
	{
		hdrs: `PUT /files/129742 HTTP/1.1\r
Host: example.com\r
User-Agent: Chrome/54.0.2803.1\r
Content-Length: 202\r
`,
		body: `This is a message body. All content in this message body should be stored under the 
/files/129742 path, as specified by the PUT specification. The message body does
not have to be terminated with CRLF.`,
		flgs: 0,
		desc: "PUT with body & content-length",
		e: pMsgExpR{
			err:    0,
			offs:   0, // auto-fill
			nHdrs:  3,
			hdrf:   HdrHostF | HdrCLenF | HdrOtherF,
			status: 0, m: MPut,
			state: MsgFIN,
		},
	},
	{
		hdrs: `GET / HTTP/1.1\r
Host: www.example.org\r
`,
		body: ``,
		flgs: 0,
		desc: "GET with no body & no content-length",
		e: pMsgExpR{
			err:    0,
			offs:   0, // auto-fill
			nHdrs:  1,
			hdrf:   HdrHostF,
			status: 0, m: MGet,
			state: MsgFIN,
		},
	},
	{
		hdrs: `GET / HTTP/1.1\r
Host: www.example.org\r
Content-Length: 0\r
`,
		body: ``,
		flgs: 0,
		desc: "GET with no body & 0 content-length",
		e: pMsgExpR{
			err:    0,
			offs:   0, // auto-fill
			nHdrs:  2,
			hdrf:   HdrHostF | HdrCLenF,
			status: 0, m: MGet,
			state: MsgFIN,
		},
	},
	{
		hdrs: `HTTP/1.1 200 OK\r
Date: Mon, 22 Mar 2004 11:15:03 GMT\r
Content-Type: text/html\r
Transfer-Encoding: chunked\r
Trailer: Expires\r
`,
		body: `28\r
<html><body><p>The file you requested is\r
5\r
3,400\r
21\r
bytes long and was last modified:\r
1d\r
Sat, 20 Mar 2004 21:12:00 GMT\r
13\r
.</p></body></html>\r
0\r
Expires: Sat, 27 Mar 2004 21:12:00 GMT\r
\r
`,
		flgs: 0,
		desc: "200 with chunked body & trailer headers",
		e: pMsgExpR{
			err:    0,
			offs:   0, // auto-fill
			nHdrs:  4,
			hdrf:   HdrTrEncodingF | HdrOtherF,
			status: 200, m: 0,
			state: MsgFIN,
		},
	},
	{
		hdrs: `PUT /test1 HTTP/1.1\r
Host: example.com\r
User-Agent: FooBar\r
Transfer-Encoding: plain, chunked\r
`,
		body: `4\r
Wiki\r
6\r
pedia \r
E\r
in \r
\r
chunks.\r
0\r
\r
`,
		flgs: 0,
		desc: "PUT with body & chunks",
		e: pMsgExpR{
			err:    0,
			offs:   0, // auto-fill
			nHdrs:  3,
			hdrf:   HdrHostF | HdrTrEncodingF | HdrOtherF,
			status: 0, m: MPut,
			state: MsgFIN,
		},
	},
	{
		hdrs: `PUT /test2 HTTP/1.1\r
Transfer-Encoding: plain\r
Host: example2.com\r
User-Agent: FooBar2\r
Transfer-Encoding: chunked  \r
`,
		body: `4\r
Wiki\r
6\r
pedia \r
E\r
in \r
\r
chunks.\r
0\r
\r
`,
		flgs: 0,
		desc: "PUT with body & chunks, 2 separate TE headers",
		e: pMsgExpR{
			err:    0,
			offs:   0, // auto-fill
			nHdrs:  4,
			hdrf:   HdrHostF | HdrTrEncodingF | HdrOtherF,
			status: 0, m: MPut,
			state: MsgFIN,
		},
	},
	{
		hdrs: `HTTP/1.1 200 OK\r
Date: Sun, 20 Oct 2021 20:20:20 GMT\r
Server: Test MsgNoMoreDataF\r
Accept-Ranges: bytes\r
Connection: close\r
Content-Type: text/html\r
`,
		body: `Hello world!`,
		flgs: MsgNoMoreDataF,
		desc: "200 with body & no content-length",
		e: pMsgExpR{
			err:    0,
			offs:   0, // auto-fill
			nHdrs:  5,
			hdrf:   HdrServerF | HdrConnectionF | HdrOtherF,
			status: 200, m: 0,
			state: MsgFIN,
		},
	},
}

func TestParseMsg(t *testing.T) {
	for _, mt := range msgTests {
		offs := 0
		mHdr := unescapeCRLF(mt.hdrs)
		mB := unescapeCRLF(mt.body)
		buf := make([]byte, len(mHdr)+2 /* crlf */ +len(mB))
		copy(buf, mHdr)
		copy(buf[len(mHdr):], []byte{'\r', '\n'})
		copy(buf[len(mHdr)+2:], mB)
		if mt.e.offs <= 0 {
			// fill expected offset: offs(first \n)+1
			// (works only if no last chunk with trailer headers)
			mt.e.offs = len(buf)
		}
		testParseMsg(t, buf, offs, &mt)
	}
}

func TestParseMsgPieces(t *testing.T) {
	for _, mt := range msgTests {
		offs := rand.Intn(100)
		mHdr := unescapeCRLF(mt.hdrs)
		mB := unescapeCRLF(mt.body)
		buf := make([]byte, offs+len(mHdr)+2 /* crlf */ +len(mB))
		copy(buf[offs:], mHdr)
		copy(buf[offs+len(mHdr):], []byte{'\r', '\n'})
		copy(buf[offs+len(mHdr)+2:], mB)
		if mt.e.offs <= 0 {
			// fill expected offset: offs(first \n)+1
			// (works only if no last chunk with trailer headers)
			mt.e.offs = len(buf)
		} else {
			mt.e.offs += offs
		}
		testParseMsgPieces(t, buf, offs, offs+len(mHdr)+2, 20, &mt)
	}
}

func testSkipBodyChunk(t *testing.T, buf []byte, tc *chTestCase1) {
	var msg PMsg
	offs := tc.o
	eOffs := len(buf)
	eErr := tc.e.err
	eState := MsgFIN
	if eErr == 0 {
		if tc.e.chkSz != 0 {
			eErr = ErrHdrMoreBytes
			eState = MsgBodyChunked
		}
	}

	// prepare fake parsed message
	msg.state = MsgBodyChunked
	msg.Body.Set(offs, offs)

	o, err := SkipBody(buf, offs, &msg, 0)

	if err != eErr {
		t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)] error %q expected",
			buf, offs, o, err, err, eErr)
	}
	if o != eOffs {
		t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)] offset %d expected",
			buf, offs, o, err, err, eOffs)
	}
	if msg.state != eState {
		t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)]"+
			" msg state %d expected, but got %d",
			buf, offs, o, err, err, msg.state, eState)
	}
}

func testSkipBodyChunkPieces(t *testing.T, buf []byte, tc *chTestCase1, n int) {
	var msg PMsg
	offs := tc.o
	eOffs := len(buf)
	eErr := tc.e.err
	eBodyOffs := tc.e.offs
	eState := MsgFIN
	if eErr == 0 {
		if tc.e.chkSz != 0 {
			eErr = ErrHdrMoreBytes
			eState = MsgBodyChunked
		}
	}

	// prepare fake parsed message
	msg.state = MsgBodyChunked
	msg.Body.Set(offs, offs)

	o := offs
	end := o // sent so far
	pieces := rand.Intn(n)
	for i := 0; i < pieces; i++ {
		psz := rand.Intn(len(buf) + 1 - end)
		if end+psz < eOffs {
			end += psz // always increase
			var err ErrorHdr
			//t.Logf("piece %d/%d: [%d:%d]: crt buf %q", i, pieces, o, end, buf[:end])
			o, err = SkipBody(buf[:end], o, &msg, 0)
			if err != ErrHdrMoreBytes {
				t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)]"+
					" error %q expected, msg state %d",
					buf[:end], offs, o, err, err, ErrHdrMoreBytes, msg.state)
			}
			tmpState := MsgBodyChunkedData
			if o < eBodyOffs || tc.e.chkSz == 0 {
				tmpState = MsgBodyChunked
			}
			if msg.state != tmpState {
				t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)]"+
					" msg state %d expected, but got %d",
					buf[:end], offs, o, err, err, msg.state, tmpState)
			}
		} else {
			break
		}
	}

	var err ErrorHdr
	o, err = SkipBody(buf, o, &msg, 0)

	if err != eErr {
		t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)] error %q expected",
			buf, offs, o, err, err, eErr)
	}
	if o != eOffs {
		t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)] offset %d expected",
			buf, offs, o, err, err, eOffs)
	}
	if msg.state != eState {
		t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)]"+
			" msg state %d expected, but got %d",
			buf, offs, o, err, err, msg.state, eState)
	}
}

func testSkipBodyChunkedBlock(t *testing.T, buf []byte, offs int) {
	var msg PMsg
	eOffs := len(buf)
	eErr := ErrHdrOk
	eState := MsgFIN

	// prepare fake parsed message
	msg.state = MsgBodyChunked
	msg.Body.Set(offs, offs)

	o, err := SkipBody(buf, offs, &msg, 0)

	if err != eErr {
		t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)] error %q expected",
			buf, offs, o, err, err, eErr)
	}
	if o != eOffs {
		t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)] offset %d expected",
			buf, offs, o, err, err, eOffs)
	}
	if msg.state != eState {
		t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)]"+
			" msg state %d expected, but got %d",
			buf, offs, o, err, err, msg.state, eState)
	}
}

func testSkipBodyChunkedBlockPieces(t *testing.T, buf []byte, offs, n int) {
	var msg PMsg
	eOffs := len(buf)
	eErr := ErrHdrOk
	eState := MsgFIN

	// prepare fake parsed message
	msg.state = MsgBodyChunked
	msg.Body.Set(offs, offs)

	o := offs
	end := o // sent so far
	pieces := rand.Intn(n)
	for i := 0; i < pieces; i++ {
		psz := rand.Intn(len(buf) + 1 - end)
		if end+psz < eOffs {
			end += psz // always increase
			var err ErrorHdr
			//t.Logf("piece %d/%d: [%d:%d]: crt buf %q", i, pieces, o, end, buf[:end])
			o, err = SkipBody(buf[:end], o, &msg, 0)
			if err != ErrHdrMoreBytes {
				t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)]"+
					" error %q expected, msg state %d",
					buf[:end], offs, o, err, err, ErrHdrMoreBytes, msg.state)
			}
		} else {
			break
		}
	}

	var err ErrorHdr
	o, err = SkipBody(buf, o, &msg, 0)

	if err != eErr {
		t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)] error %q expected",
			buf, offs, o, err, err, eErr)
	}
	if o != eOffs {
		t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)] offset %d expected",
			buf, offs, o, err, err, eOffs)
	}
	if msg.state != eState {
		t.Errorf("SkipBody(%q, %d, ...) = [ %d, %d (%q)]"+
			" msg state %d expected, but got %d",
			buf, offs, o, err, err, msg.state, eState)
	}
}

func testParseMsg(t *testing.T, buf []byte, offs int, mt *pMsgTestCase) {
	var msg PMsg

	o, err := ParseMsg(buf, offs, &msg, mt.flgs)
	if err != mt.e.err {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			" error %q expected",
			buf, offs, mt.flgs, o, err, err, mt.e.err)
	}
	if o != mt.e.offs {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			" offset %d expected",
			buf, offs, mt.flgs, o, err, err, mt.e.offs)
	}
	if msg.state != mt.e.state {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			" msg state %d expected, but got %d",
			buf, offs, mt.flgs, o, err, err, mt.e.state, msg.state)
	}
	if mt.e.status > 0 && msg.FL.Status != mt.e.status {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			" msg status %d expected, but got %d",
			buf, offs, mt.flgs, o, err, err, mt.e.status, msg.FL.Status)
	}
	if mt.e.m > 0 && msg.FL.MethodNo != mt.e.m {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			" msg method %d (%s) expected, but got %d (%s)",
			buf, offs, mt.flgs, o, err, err, mt.e.m, mt.e.m,
			msg.FL.Method, msg.FL.MethodNo)
	}
	if mt.e.nHdrs > 0 && msg.HL.N != mt.e.nHdrs {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			"  %d headers expected, but got %d",
			buf, offs, mt.flgs, o, err, err, mt.e.nHdrs, msg.HL.N)
	}
	if mt.e.hdrf > 0 && msg.HL.PFlags != mt.e.hdrf {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			"  0x%x parsed header flags expected, but got 0x%x",
			buf, offs, mt.flgs, o, err, err, mt.e.hdrf, msg.HL.PFlags)
	}
}

func testParseMsgPieces(t *testing.T, buf []byte, offs int, bodyOffs int,
	n int, mt *pMsgTestCase) {
	var msg PMsg

	o := offs
	end := o // sent so far
	pieces := rand.Intn(n)
	for i := 0; i < pieces; i++ {
		psz := rand.Intn(len(buf) + 1 - end)
		if end+psz < len(buf) {
			end += psz // always increase
			var err ErrorHdr
			//t.Logf("piece %d/%d: [%d:%d]: crt buf %q", i, pieces, o, end, buf[offs:end])
			flgs := mt.flgs
			if flgs&MsgNoMoreDataF != 0 && end >= bodyOffs {
				// if MsgNoMoreDataF & current piece includes body
				// force reset MsgNoMoreDataF for this piece
				// (otherwise we'll end with a body-truncated message if
				//  the msg has no content-length or chunked tr-enc)
				flgs &= ^uint8(MsgNoMoreDataF)
			}
			o, err = ParseMsg(buf[:end], o, &msg, flgs)
			if flgs&MsgNoMoreDataF != 0 {
				if err != ErrHdrTrunc {
					t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
						" error %q expected",
						buf[:end], offs, mt.flgs, o, err, err, ErrHdrTrunc)
				}
			} else if err != ErrHdrMoreBytes {
				t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
					" error %q expected",
					buf[:end], offs, mt.flgs, o, err, err, ErrHdrMoreBytes)
			}
		} else {
			break
		}
	}

	var err ErrorHdr
	o, err = ParseMsg(buf, o, &msg, mt.flgs)
	if err != mt.e.err {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			" error %q expected",
			buf, offs, mt.flgs, o, err, err, mt.e.err)
	}
	if o != mt.e.offs {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			" offset %d expected",
			buf, offs, mt.flgs, o, err, err, mt.e.offs)
	}
	if msg.state != mt.e.state {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			" msg state %d expected, but got %d",
			buf, offs, mt.flgs, o, err, err, mt.e.state, msg.state)
	}
	if mt.e.status > 0 && msg.FL.Status != mt.e.status {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			" msg status %d expected, but got %d",
			buf, offs, mt.flgs, o, err, err, mt.e.status, msg.FL.Status)
	}
	if mt.e.m > 0 && msg.FL.MethodNo != mt.e.m {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			" msg method %d (%s) expected, but got %d (%s)",
			buf, offs, mt.flgs, o, err, err, mt.e.m, mt.e.m,
			msg.FL.Method, msg.FL.MethodNo)
	}
	if mt.e.nHdrs > 0 && msg.HL.N != mt.e.nHdrs {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			"  %d headers expected, but got %d",
			buf, offs, mt.flgs, o, err, err, mt.e.nHdrs, msg.HL.N)
	}
	if mt.e.hdrf > 0 && msg.HL.PFlags != mt.e.hdrf {
		t.Errorf("ParseMsg(%q, %d, ... 0x%x) = [ %d, %d (%q)]"+
			"  0x%x parsed header flags expected, but got 0x%x",
			buf, offs, mt.flgs, o, err, err, mt.e.hdrf, msg.HL.PFlags)
	}
}
