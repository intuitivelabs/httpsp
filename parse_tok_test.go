// Copyright 2021 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

package httpsp

import (
	"testing"

	"bytes"
)

func TestParseTokLst(t *testing.T) {

	type testCase struct {
		t     []byte   // test string
		offs  int      // offset in t
		flags uint     // parsing flags
		eN    int      // expected token number
		eToks []string // expected tokens
		eOffs int      // expected offset
		eErr  ErrorHdr // expected error
	}

	tests := [...]testCase{
		{t: []byte("foo\r\nX"), offs: 0,
			eN: 1, eToks: []string{"foo"},
			eOffs: 5, eErr: ErrHdrOk},
		{t: []byte(" foo \r\nX"), offs: 0,
			eN: 1, eToks: []string{"foo"},
			eOffs: 7, eErr: ErrHdrOk},
		{t: []byte("foo bar\r\nX"), offs: 0,
			eN: 2, eToks: []string{"foo", "bar"},
			eOffs: 4, eErr: ErrHdrBadChar}, // fail on "b" - multiple tokens
		{t: []byte("foo1 bar\r\nX"), offs: 0, flags: PTokSpSepF,
			eN: 2, eToks: []string{"foo1", "bar"},
			eOffs: 10, eErr: ErrHdrOk}, // ok: flags allow WS
		{t: []byte("foo2 bar\r\nX"), offs: 0, flags: PTokCommaSepF,
			eN: 2, eToks: []string{"foo2", "bar"},
			eOffs: 5, eErr: ErrHdrBadChar}, // fail, flags doesn't allow WS
		{t: []byte("foo3,bar\r\nX"), offs: 0, flags: PTokCommaSepF,
			eN: 2, eToks: []string{"foo3", "bar"},
			eOffs: 10, eErr: ErrHdrOk}, // ok: flags allow ','
		{t: []byte("foo4, bar\r\nX"), offs: 0, flags: PTokCommaSepF,
			eN: 2, eToks: []string{"foo4", "bar"},
			eOffs: 11, eErr: ErrHdrOk}, // ok: flags allow ','
		{t: []byte("foo5 , bar\r\nX"), offs: 0, flags: PTokCommaSepF,
			eN: 2, eToks: []string{"foo5", "bar"},
			eOffs: 12, eErr: ErrHdrOk}, // ok: flags allow ','
		{t: []byte("foo6 , bar baz\r\nX"), offs: 0, flags: PTokCommaSepF,
			eN: 2, eToks: []string{"foo6", "bar", "baz"},
			eOffs: 11, eErr: ErrHdrBadChar}, // err: space not allowed as sep
		{t: []byte("foo7 , bar baz\r\nX"), offs: 0,
			flags: PTokCommaSepF | PTokSpSepF,
			eN:    3, eToks: []string{"foo7", "bar", "baz"},
			eOffs: 16, eErr: ErrHdrOk}, // ok: space & ',' allowed
		{t: []byte(" foo8 , ,,, , bar ,  	 , baz ,, , 	,\r\nX"), offs: 0,
			flags: PTokCommaSepF | PTokSpSepF,
			eN:    3, eToks: []string{"foo8", "bar", "baz"},
			eOffs: -1, eErr: ErrHdrOk}, // ok: space & ',' allowed
		{t: []byte("foo9"), offs: 0,
			eN: 1, eToks: []string{"foo"},
			eOffs: 4, eErr: ErrHdrMoreBytes},
		{t: []byte("foo10, bar\r\n\r\n"), offs: 0, flags: PTokCommaSepF,
			eN: 2, eToks: []string{"foo10", "bar"},
			eOffs: 12, eErr: ErrHdrOk}, // ok: flags allow ','
		{t: []byte("foo11, bar\r\n"), offs: 0, flags: PTokCommaSepF,
			eN: 2, eToks: []string{"foo11", "bar"},
			eOffs: 10, eErr: ErrHdrMoreBytes}, // ok: flags allow ','
		{t: []byte("foo12, bar/1.0\r\n\r\n"), offs: 0, flags: PTokCommaSepF,
			eN: 2, eToks: []string{"foo12", "bar"},
			eOffs: 10, eErr: ErrHdrBadChar}, // erR: bad char '/'
		{t: []byte("foo13, bar/1.0\r\n\r\n"), offs: 0,
			flags: PTokCommaSepF | PTokAllowSlashF,
			eN:    2, eToks: []string{"foo13", "bar/1.0"},
			eOffs: 16, eErr: ErrHdrOk}, // erR: bad char '/'
	}

	for _, tc := range tests {
		// fix wild card values
		if tc.eOffs <= 0 {
			// assume \r\n + extra char
			tc.eOffs = len(tc.t) - 1
		}

		var tok PToken
		var err ErrorHdr
		n := 0
		o := tc.offs
		for {
			tok.Reset()
			o, err = ParseTokenLst(tc.t, o, &tok, tc.flags)
			if err != ErrHdrMoreValues && err != ErrHdrOk {
				break
			}
			if n < len(tc.eToks) {
				if bytes.Compare(tok.V.Get(tc.t), []byte(tc.eToks[n])) != 0 {
					t.Errorf("TestParseTokLst: token %d do not match:"+
						" %q != %q (expected) for %q", n,
						tok.V.Get(tc.t), tc.eToks[n], tc.t)
				}
			}
			n++
			if n > tc.eN {
				t.Errorf("TestParseTokLst: too many token found %d,"+
					" expected %d (%q)", n, tc.eN, tc.t)
				break
			}
			if err != ErrHdrMoreValues {
				break
			}
			t.Logf("test %q, next token %d, offset %d ('%c') err %q",
				tc.t, n, o, tc.t[o], err)
		}
		if err != tc.eErr {
			t.Errorf("TestParseTokLst: error code mismatch: %d (%q),"+
				" expected %d (%q) for %q @%d ('%c') (%d/%d tokens found)",
				err, err, tc.eErr, tc.eErr, tc.t, o, tc.t[o], n, tc.eN)
		} else if o != tc.eOffs {
			t.Errorf("TestParseTokLst: offset mismatch: %d,"+
				" expected %d for %q (%d/%d tokens found)",
				o, tc.eOffs, tc.t, n, tc.eN)
		}
	}
}
