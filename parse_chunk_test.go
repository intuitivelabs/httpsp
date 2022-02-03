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

type expR1 struct {
	err   ErrorHdr
	offs  int
	chkSz int64
	nHdrs int
}

type chTestCase1 struct {
	chHdr  string
	chData string
	o      int
	e      expR1
}

var chunkTests = [...]chTestCase1{
	// from https://en.wikipedia.org/wiki/Chunked_transfer_encoding
	{chHdr: "4\r\n", chData: "Wiki\r\n",
		e: expR1{err: 0, offs: 3, chkSz: 4, nHdrs: 0}},
	{chHdr: "6\r\n", chData: "pedia \r\n",
		e: expR1{err: 0, offs: 0, chkSz: -1, nHdrs: 0}},
	{chHdr: "E\r\n", chData: "in \r\n\r\nchunks.\r\n",
		e: expR1{err: 0, offs: 0, chkSz: -1, nHdrs: 0}},
	{chHdr: "000e\r\n", chData: "in \r\n\r\nchunks.\r\n",
		e: expR1{err: 0, offs: 0, chkSz: -1, nHdrs: 0}},
	{chHdr: "0000000e\r\n", chData: "in \r\n\r\nchunks.\r\n",
		e: expR1{err: 0, offs: 0, chkSz: -1, nHdrs: 0}},
	{chHdr: "0\r\n", chData: "\r\n",
		e: expR1{err: 0, offs: 0, chkSz: 0, nHdrs: 0}},
	{chHdr: "0000\r\n", chData: "\r\n",
		e: expR1{err: 0, offs: 0, chkSz: 0, nHdrs: 0}},
	{chHdr: "0000000\r\n", chData: "\r\n",
		e: expR1{err: 0, offs: 0, chkSz: 0, nHdrs: 0}},
	{chHdr: "00000000000000000\r\n", chData: "\r\n",
		e: expR1{err: 0, offs: 0, chkSz: 0, nHdrs: 0}},
	// from:
	// https://wiki.wireshark.org/uploads/__moin_import__/attachments/SampleCaptures/http-chunked-gzip.pcap
	{chHdr: "\x66\x0d\x0a",
		chData: "\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\x03\x00" +
			"\x00\x00\xff\xff\x0d\x0a",
		e: expR1{err: 0, chkSz: 15, nHdrs: 0}},
	// final chunk:
	{chHdr: "0\r\n" +
		"Test-Hdr: foo bar\r\n",
		chData: "\r\n",
		e:      expR1{err: 0, offs: 22, chkSz: 0, nHdrs: 1}},
	// final chunk more hdrs:
	{chHdr: "0\r\n" +
		"Foo: header1\r\n" +
		"Bar: header2\r\n",
		chData: "\r\n",
		e:      expR1{err: 0, offs: 31, chkSz: 0, nHdrs: 2}},
}

func TestParseChunk1(t *testing.T) {

	var cv ChunkVal
	for _, c := range chunkTests {
		chunkHdr := unescapeCRLF(c.chHdr)
		chunkD := unescapeCRLF(c.chData)
		if c.e.offs <= 0 {
			// fill expected offset: offs(first \n)+1
			// (works only if no last chunk with trailer headers)
			c.e.offs = len(chunkHdr)
		}
		if c.e.chkSz < 0 {
			// fill expected chunk size : data size - 2 ("\r\n")
			// (works only if no last chunk with trailer headers)
			c.e.chkSz = int64(len(chunkD) - 2)
		}
		testParseChunk(t, chunkHdr, chunkD, c.o, &cv, &c)
		cv.Reset()
	}
}

func TestParseChunkPieces(t *testing.T) {

	var cv ChunkVal
	for _, c := range chunkTests {
		chunkHdr := unescapeCRLF(c.chHdr)
		chunkD := unescapeCRLF(c.chData)
		if c.e.offs <= 0 {
			// fill expected offset: offs(first \n)+1
			// (works only if no last chunk with trailer headers)
			c.e.offs = len(chunkHdr)
		}
		if c.e.chkSz < 0 {
			// fill expected chunk size : data size - 2 ("\r\n")
			// (works only if no last chunk with trailer headers)
			c.e.chkSz = int64(len(chunkD) - 2)
		}
		testParseChunkPieces(t, chunkHdr, chunkD, c.o, &cv, &c, 10)
		cv.Reset()
	}
}

func testParseChunk(t *testing.T, chHdr, chD []byte, offs int, cv *ChunkVal,
	tc *chTestCase1) {
	buf := make([]byte, len(chHdr)+len(chD))
	copy(buf, chHdr)
	copy(buf[len(chHdr):], chD)

	o, sz, err := ParseChunk(buf, offs, cv)

	if err != tc.e.err {
		t.Errorf("ParseChunk(%q, %d, ..) = [ %d, %d, %d(%q)]"+
			" error %q expected", buf, offs, o, sz, err, err, tc.e.err)
	}
	if o != tc.e.offs {
		t.Errorf("ParseChunk(%q, %d, ..) = [ %d, %d, %d(%q)]"+
			" offset %d expected", buf, offs, o, sz, err, err, tc.e.offs)
	}
	if sz != tc.e.chkSz {
		t.Errorf("ParseChunk(%q, %d, ..) = [ %d, %d, %d(%q)]"+
			" return size %d expected", buf, offs, o, sz, err, err, tc.e.chkSz)
	}
	if cv.Size != tc.e.chkSz {
		t.Errorf("ParseChunk(%q, %d, ..) = [ %d, %d, %d(%q)]"+
			" got chunk size %d, but %d expected",
			buf, offs, o, sz, err, err, cv.Size, tc.e.chkSz)
	}
	if cv.TrailerHdrs.N != tc.e.nHdrs {
		t.Errorf("ParseChunk(%q, %d, ..) = [ %d, %d, %d(%q)]"+
			" got  %d trailer headers, but %d expected",
			buf, offs, o, sz, err, err, cv.TrailerHdrs.N, tc.e.nHdrs)
	}
}

func testParseChunkPieces(t *testing.T, chHdr, chD []byte,
	offs int, cv *ChunkVal, tc *chTestCase1, n int) {

	buf := make([]byte, len(chHdr)+len(chD))
	copy(buf, chHdr)
	copy(buf[len(chHdr):], chD)

	o := offs
	pieces := rand.Intn(n)
	for i := 0; i < pieces; i++ {
		psz := rand.Intn(len(chHdr) + 1 - o)
		end := psz + o
		if end < tc.e.offs {
			var sz int64
			var err ErrorHdr
			o, sz, err = ParseChunk(buf[:end], o, cv)
			if err != ErrHdrMoreBytes {
				t.Errorf("ParseChunkPieces(%q, %d, ..) = [ %d, %d, %d(%q)]"+
					" error %q expected, state %d",
					buf, offs, o, sz, err, err, ErrHdrMoreBytes, cv.state)
			}
		} else {
			break
		}
	}
	testParseChunk(t, chHdr, chD, o, cv, tc)
}
