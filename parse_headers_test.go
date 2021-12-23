// Copyright 2021 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

package httpsp

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"unsafe"
)

func TestHdrNameLookup(t *testing.T) {
	// statistics
	var max, crowded, total int
	for _, l := range hdrNameLookup {
		if len(l) > max {
			max = len(l)
		}
		if len(l) > 1 {
			crowded++
		}
		total += len(l)
	}
	if total != len(hdrName2Type) {
		t.Errorf("init: hdrNameLookup[%d][..]:"+
			" lookup hash has too few elements %d/%d (max %d, crowded %d)\n",
			len(hdrNameLookup), total, len(hdrName2Type), max, crowded)
	}
	if max > 2 {
		t.Errorf("init: hdrNameLookup[%d][..]: max %d, crowded %d, total %d"+
			" - try increasing hnBitsLen(%d) and/or hnBitsFChar(%d)\n",
			len(hdrNameLookup), max, crowded, total, hnBitsLen, hnBitsFChar)
	}
	if max > 0 {
		t.Logf("init: hdrNameLookup[%d][..]: max %d, crowded %d, total %d\n",
			len(hdrNameLookup), max, crowded, total)
	}
}

func TestHdrFlags(t *testing.T) {
	var f HdrFlags
	if unsafe.Sizeof(f)*8 <= uintptr(HdrOther) {
		t.Errorf("HdrFlags: flags type too small: %d bits but %d needed\n",
			unsafe.Sizeof(f)*8, HdrOther)
	}
	for h := HdrNone; h <= HdrOther; h++ {
		f.Set(h)
		if !f.Test(h) {
			t.Errorf("HdrFlags.Test(%v): wrong return\n", f)
		}
	}
	for h := HdrNone; h <= HdrOther; h++ {
		f.Clear(h)
		if f.Test(h) {
			t.Errorf("HdrFlags.Test(%v): wrong return\n", f)
		}
	}

}

func TestHdr2Str(t *testing.T) {
	if len(hdrTStr) != (int(HdrOther) + 1) {
		t.Errorf("hdrTStr[]: length mismatch %d/%d\n",
			len(hdrTStr), int(HdrOther)+1)
	}
	for i, v := range hdrTStr {
		if len(v) == 0 {
			t.Errorf("hdrTStr[%d]: empty name\n", i)
		}
	}
	for h := HdrNone; h <= HdrOther; h++ {
		if len(h.String()) == 0 || strings.EqualFold(h.String(), "invalid") {
			t.Errorf("header type %d has invalid string value %q\n",
				h, h.String())
		}
	}

}

type eRes struct {
	err  ErrorHdr
	offs int
	t    HdrT
	hn   []byte
	hv   []byte
}

type testCase struct {
	n string // header name (without ':')
	b string // header body (without CRLF)
	eRes
}

var testsHeaders = [...]testCase{
	{n: "Content-Length", b: "12345", eRes: eRes{err: 0, t: HdrCLen}},
	{n: "Transfer-Encoding", b: "chunked",
		eRes: eRes{err: 0, t: HdrTrEncoding}},
	{n: "Transfer-Encoding", b: "gzip, chunked",
		eRes: eRes{err: 0, t: HdrTrEncoding}},
	{n: "Upgrade", b: "websocket", eRes: eRes{err: 0, t: HdrUpgrade}},
	{n: "Upgrade", b: "HTTP/2.0, SHTTP/1.3,  IRC/6.9,   RTA/x11",
		eRes: eRes{err: 0, t: HdrUpgrade}},
	{n: "Content-Encoding", b: "deflate", eRes: eRes{err: 0, t: HdrCEncoding}},
	{n: "Content-Encoding", b: "deflate,  gzip",
		eRes: eRes{err: 0, t: HdrCEncoding}},
	{n: "Host", b: "foo.bar", eRes: eRes{err: 0, t: HdrHost}},
	{n: "Host", b: "locahost:8080", eRes: eRes{err: 0, t: HdrHost}},
	{n: "Server", b: "Apache/2.0.0 (Unix)", eRes: eRes{err: 0, t: HdrServer}},
	{n: "Server", b: "Foo 	Bar 5.0", eRes: eRes{err: 0, t: HdrServer}},
	{n: "Origin", b: "null", eRes: eRes{err: 0, t: HdrOrigin}},
	{n: "Origin", b: "http://foo.bar:8080", eRes: eRes{err: 0, t: HdrOrigin}},
	{n: "Connection", b: "Upgrade", eRes: eRes{err: 0, t: HdrConnection}},
	{n: "Sec-WebSocket-Key", b: "dGhlIHNhbXBsZSBub25jZQ==",
		eRes: eRes{err: 0, t: HdrWSockKey}},
	{n: "Sec-WebSocket-Protocol", b: "sip",
		eRes: eRes{err: 0, t: HdrWSockProto}},
	{n: "Sec-WebSocket-Protocol", b: "chat, superchat",
		eRes: eRes{err: 0, t: HdrWSockProto}},
	{n: "Sec-WebSocket-Accept", b: "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=",
		eRes: eRes{err: 0, t: HdrWSockAccept}},
	{n: "Sec-WebSocket-Version", b: "13",
		eRes: eRes{err: 0, t: HdrWSockVer}},
	{n: "Foo", b: "generic header", eRes: eRes{err: 0, t: HdrOther}},
}

func TestParseHdrLine(t *testing.T) {

	var b []byte
	ws := [...][3]string{
		{"", "", ""},
		{"", " ", ""},
		{" ", " ", " "},
	}
	tests := testsHeaders

	for i := 0; i < (len(ws) + 2); i++ {
		for _, c := range tests {
			var ws1, lws, lwsE, n string
			if i < len(ws) {
				ws1 = ws[i][0]
				lws = ws[i][1]
				lwsE = ws[i][2]
			} else {
				ws1 = randWS()
				lws = randLWS()
				lwsE = randLWS()
			}
			if i%2 == 1 {
				n = randCase(c.n)
			} else {
				n = c.n
			}
			b = []byte(n + ws1 + ":" + lws + c.b + lwsE + "\r\n\r\n")
			c.offs = len(b) - 2
			c.hn = []byte(n)
			c.hv = []byte(c.b)
			var hdr Hdr
			var phvals PHdrVals
			testParseHdrLine(t, b, 0, &hdr, nil, &c.eRes)
			hdr.Reset()
			testParseHdrLine(t, b, 0, &hdr, &phvals, &c.eRes)
			testParseHdrLinePieces(t, b, 0, &c.eRes, 10)
		}
	}
}

func testParseHdrLine(t *testing.T, buf []byte, offs int, hdr *Hdr, phb PHBodies, e *eRes) {

	var err ErrorHdr
	o := offs
	o, err = ParseHdrLine(buf, o, hdr, phb)
	if err != e.err {
		t.Errorf("ParseHdrLine(%q, %d, ..)=[%d, %d(%q)]  error %s (%q) expected, state %d, hdr %q arround %q",
			buf, offs, o, err, err, e.err, e.err, hdr.state, hdr.Type, buf[o:])
	}
	if o != e.offs {
		t.Errorf("ParseHdrLine(%q, %d, ..)=[%d, %d(%q)]  offset %d expected, state %d",
			buf, offs, o, err, err, e.offs, hdr.state)
	}
	if err != 0 {
		return
	}
	if hdr.Type != e.t {
		t.Errorf("ParseHdrLine(%q, %d, ..)=[%d, %d(%q)]  type %d %q != %d %q (exp), state %d",
			buf, offs, o, err, err, hdr.Type, hdr.Type, e.t, e.t, hdr.state)
	}

	if !bytes.Equal(e.hn, hdr.Name.Get(buf)) {
		t.Errorf("ParseHdrLine(%q, %d, ..)=[%d, %d(%q)]  hdr name %q !=  %q (exp), state %d",
			buf, offs, o, err, err, hdr.Name.Get(buf), e.hn, hdr.state)
	}
	if !bytes.Equal(e.hv, hdr.Val.Get(buf)) {
		t.Errorf("ParseHdrLine(%q, %d, ..)=[%d, %d(%q)]  hdr val %q !=  %q (exp), state %d",
			buf, offs, o, err, err, hdr.Val.Get(buf), e.hv, hdr.state)
	}
}

func testParseHdrLinePieces(t *testing.T, buf []byte, offs int, e *eRes, n int) {
	var err ErrorHdr
	var hdr Hdr
	var phvals PHdrVals
	o := offs
	pieces := rand.Intn(n)
	for i := 0; i < pieces; i++ {
		sz := rand.Intn(len(buf) + 1 - o)
		end := sz + o
		if end < e.offs {
			o, err = ParseHdrLine(buf[:end], o, &hdr, &phvals)
			if err != ErrHdrMoreBytes {
				t.Errorf("ParseHdrLine partial (%q, %d, ..)=[%d, %d(%q)] "+
					"  error %s (%q) expected, state %d",
					buf, offs, o, err, err, ErrHdrMoreBytes, ErrHdrMoreBytes,
					hdr.state)
			}
		} else {
			break
		}
	}
	testParseHdrLine(t, buf, o, &hdr, &phvals, e)
}

type mTest struct {
	m    string
	err  ErrorHdr
	n    int
	offs int
}

func TestParseHeaders(t *testing.T) {
	tests := [...]mTest{
		{m: `Upgrade: websocket
Origin: null
Host: foo.bar
Server: Foo Bar 1.0
Via: HTTP/1.1 foo1.bar
Transer-Encoding: gzip
Date: Thu, 21 Feb 2002 13:02:03 GMT
Content-Length: 568
`, n: 8},
		{m: `Host: sip-ws.example.com
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: anIBJedaWX14ZSBubABiPR==
Origin: http://www.example.com
Sec-WebSocket-Protocol: sip
Sec-WebSocket-Version: 13
`, n: 7},
		{m: `Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
Sec-WebSocket-Protocol: chat
`, n: 4},
	}
	var hl HdrLst
	var hdrs [20]Hdr
	var phv PHdrVals

	offs := 0
	hl.Hdrs = hdrs[:]
	for _, c := range tests {
		//buf := []byte(c.m + "\r\n")
		buf := make([]byte, len(c.m)+2) // space for extra \r\n
		// handle escapes: \r & \n
		var i int
		var escape bool
		for _, b := range []byte(c.m) {
			if b == '\\' {
				escape = true
				continue
			} else {
				if escape {
					switch b {
					case 'n':
						buf[i] = '\n'
					case 'r':
						buf[i] = '\r'
					case 't':
						buf[i] = '\t'
					case '\\':
						buf[i] = '\\'
					default:
						// unrecognized => \char
						buf[i] = '\\'
						i++
						buf[i] = b
					}
					escape = false
				} else {
					buf[i] = b
				}
			}
			i++
		}
		buf[i] = '\r'
		buf[i+1] = '\n'
		buf = buf[:i+2]

		if c.offs == 0 {
			c.offs = len(buf)
		}
		testParseHeaders(t, buf, offs, &hl, &phv, &c)
		// debugging
		/*
			for i, h := range hl.Hdrs {
				if i >= hl.N {
					break
				}
				t.Logf("H%2d %q : %q [%q]\n",
					i, h.Name.Get(buf), h.Val.Get(buf), h.Type)
			}
		*/
		hl.Reset()
		testParseHeadersPieces(t, buf, offs, &hl, &phv, &c, 20)
		hl.Reset()
	}
}

func testParseHeaders(t *testing.T, buf []byte, offs int, hl *HdrLst, hb PHBodies, e *mTest) {
	o, err := ParseHeaders(buf, offs, hl, hb)
	if err != e.err {
		t.Errorf("ParseHeaders(%q, %d, ..)=[%d, %d(%q)]  error %s (%q) expected",
			buf, offs, o, err, err, e.err, e.err)
	}
	if o != e.offs {
		t.Errorf("ParseHeaders(%q, %d, ..)=[%d, %d(%q)]  offset %d expected",
			buf, offs, o, err, err, e.offs)
	}
	if hl.N != e.n {
		t.Errorf("ParseHeaders(%q, %d, ..)=[%d, %d(%q)] %d headers instead of %d ",
			buf, offs, o, err, err, hl.N, e.n)
	}
}

func testParseHeadersPieces(t *testing.T, buf []byte, offs int, hl *HdrLst, hb PHBodies, e *mTest, n int) {

	var err ErrorHdr
	o := offs
	pieces := rand.Intn(n)
	for i := 0; i < pieces; i++ {
		sz := rand.Intn(len(buf) + 1 - o)
		end := sz + o
		if end < e.offs {
			o, err = ParseHeaders(buf[:end], o, hl, hb)
			if err != ErrHdrMoreBytes {
				t.Errorf("ParseHeaders partial (%q, %d, ..)=[%d, %d(%q)] "+
					"  error %s (%q) expected",
					buf, offs, o, err, err, ErrHdrMoreBytes, ErrHdrMoreBytes)
			}
		} else {
			break
		}
	}
	testParseHeaders(t, buf, o, hl, hb, e)
}
