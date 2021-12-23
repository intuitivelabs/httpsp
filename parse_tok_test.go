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

func TestSkipQuoted(t *testing.T) {
	type testCase struct {
		t     []byte   // test string
		offs  int      // offset in t
		eOffs int      // expected offset
		eErr  ErrorHdr // expected error
	}

	tests := [...]testCase{
		{t: []byte("q1 bar\""), offs: 0, eOffs: 7, eErr: ErrHdrOk},
		{t: []byte("q2 \\\" \\\" bar\""), offs: 0, eOffs: 13, eErr: ErrHdrOk},
		{t: []byte("q3 bar"), offs: 0, eOffs: 6, eErr: ErrHdrMoreBytes},
		{t: []byte("q4 bar\\"), offs: 0, eOffs: 6, eErr: ErrHdrMoreBytes},
		{t: []byte("q5 bar\n"), offs: 0, eOffs: 6, eErr: ErrHdrBadChar},
		{t: []byte("q6\rbar"), offs: 0, eOffs: 2, eErr: ErrHdrBadChar},
		{t: []byte("q6\\\nbar"), offs: 0, eOffs: 3, eErr: ErrHdrBadChar},
		{t: []byte("q5 bar\\\r"), offs: 0, eOffs: 7, eErr: ErrHdrBadChar},
	}

	for _, tc := range tests {
		var err ErrorHdr
		var nxtChr string
		o := tc.offs
		o, err = SkipQuoted(tc.t, o)

		if o == len(tc.t) {
			nxtChr = "EOF" // place holder for end of input
		} else if o > len(tc.t) {
			nxtChr = "ERR_OVERFLOW" // place holder for out of buffer
		} else {
			nxtChr = string(tc.t[o])
		}
		if err != tc.eErr {
			t.Errorf("TestSkipQuoted: error code mismatch: %d (%q),"+
				" expected %d (%q) for %q @%d ('%s')",
				err, err, tc.eErr, tc.eErr, tc.t, o, nxtChr)
		} else if o != tc.eOffs {
			t.Errorf("TestParseTokLst: offset mismatch: %d,"+
				" expected %d for %q",
				o, tc.eOffs, tc.t)
		}
	}
}

func TestParseTokenParam(t *testing.T) {
	type testCase struct {
		t     []byte   // test string
		offs  int      // offset in t
		flags uint     // parsing flags
		eAll  string   // expected trimmed param
		eName string   // expected trimmed param name
		eVal  string   // expected trimmed param value
		eOffs int      // expected offset
		eErr  ErrorHdr // expected error
	}

	tests := [...]testCase{
		{t: []byte("p1\r\nX"), offs: 0, flags: 0,
			eAll: "p1", eName: "p1", eVal: "",
			eOffs: 4, eErr: ErrHdrEOH},
		{t: []byte("p2=v2\r\nX"), offs: 0, flags: 0,
			eAll: "p2=v2", eName: "p2", eVal: "v2",
			eOffs: 7, eErr: ErrHdrEOH},
		{t: []byte(" p3	 \r\nX"), offs: 0, flags: 0,
			eAll: "p3", eName: "p3", eVal: "",
			eOffs: 7, eErr: ErrHdrEOH},
		{t: []byte("	 p5 = v5 \r\nX"), offs: 0, flags: 0,
			eAll: "p5 = v5", eName: "p5", eVal: "v5",
			eOffs: 12, eErr: ErrHdrEOH},
		{t: []byte("p6=v6;foo=bar\r\nX"), offs: 0, flags: 0,
			eAll: "p6=v6", eName: "p6", eVal: "v6",
			eOffs: 6, eErr: ErrHdrMoreValues},
		{t: []byte("p7=v7;foo=bar\r\nX"), offs: 6, flags: 0,
			eAll: "foo=bar", eName: "foo", eVal: "bar",
			eOffs: 15, eErr: ErrHdrEOH},
		{t: []byte(" p8 = v8 ; foo = bar\r\nX"), offs: 0, flags: 0,
			eAll: "p8 = v8", eName: "p8", eVal: "v8",
			eOffs: 11, eErr: ErrHdrMoreValues},
		{t: []byte("p9=v9,foo\r\nX"), offs: 0, flags: PTokCommaSepF,
			eAll: "p9=v9", eName: "p9", eVal: "v9",
			eOffs: 5, eErr: ErrHdrOk},
		{t: []byte("p10=v10 foo\r\nX"), offs: 0, flags: PTokSpSepF,
			eAll: "p10=v10", eName: "p10", eVal: "v10",
			eOffs: 7, eErr: ErrHdrOk},
		{t: []byte("p11=v11   foo\r\nX"), offs: 0, flags: PTokSpSepF,
			eAll: "p11=v11", eName: "p11", eVal: "v11",
			eOffs: 9, eErr: ErrHdrOk},
		{t: []byte("p12=v12 , foo\r\nX"), offs: 0,
			flags: PTokCommaSepF | PTokSpSepF,
			eAll:  "p12=v12", eName: "p12", eVal: "v12",
			eOffs: 8, eErr: ErrHdrOk},
		{t: []byte("p13;foo=bar\r\nX"), offs: 0, flags: 0,
			eAll: "p13", eName: "p13", eVal: "",
			eOffs: 4, eErr: ErrHdrMoreValues},
		{t: []byte("p14,foo=bar\r\nX"), offs: 0, flags: PTokCommaSepF,
			eAll: "p14", eName: "p14", eVal: "",
			eOffs: 3, eErr: ErrHdrOk},
		{t: []byte("p15 foo=bar\r\nX"), offs: 0, flags: PTokSpSepF,
			eAll: "p15", eName: "p15", eVal: "",
			eOffs: 3, eErr: ErrHdrOk},
		{t: []byte("p15 ; foo=bar\r\nX"), offs: 0, flags: PTokSpSepF,
			eAll: "p15", eName: "p15", eVal: "",
			eOffs: 6, eErr: ErrHdrMoreValues},
		{t: []byte("p16\r\n"), offs: 0, flags: PTokInputEndF,
			eAll: "p16", eName: "p16", eVal: "",
			eOffs: 5, eErr: ErrHdrEOH},
		{t: []byte("p17"), offs: 0, flags: PTokInputEndF,
			eAll: "p17", eName: "p17", eVal: "",
			eOffs: 3, eErr: ErrHdrEOH},
		{t: []byte("p18=v18\r\n"), offs: 0, flags: PTokInputEndF,
			eAll: "p18=v18", eName: "p18", eVal: "v18",
			eOffs: 9, eErr: ErrHdrEOH},
		{t: []byte("p19=v19"), offs: 0, flags: PTokInputEndF,
			eAll: "p19=v19", eName: "p19", eVal: "v19",
			eOffs: 7, eErr: ErrHdrEOH},
		{t: []byte("test.foo.bar\r\nX"), offs: 0, flags: 0,
			eAll: "test.foo.bar", eName: "test.foo.bar", eVal: "",
			eOffs: 14, eErr: ErrHdrEOH},
		{t: []byte("test-1-2.foo.bar\r\nX"), offs: 0, flags: 0,
			eAll: "test-1-2.foo.bar", eName: "test-1-2.foo.bar", eVal: "",
			eOffs: 18, eErr: ErrHdrEOH},
	}

	var param PTokParam
	for _, tc := range tests {
		var err ErrorHdr
		var nxtChr string
		param.Reset()
		o := tc.offs

		o, err = ParseTokenParam(tc.t, o, &param, tc.flags)

		if o == len(tc.t) {
			nxtChr = "EOF" // place holder for end of input
		} else if o > len(tc.t) {
			nxtChr = "ERR_OVERFLOW" // place holder for out of buffer
		} else {
			nxtChr = string(tc.t[o])
		}

		if err != tc.eErr {
			t.Errorf("TestParseTokenParam: error code mismatch: %d (%q),"+
				" expected %d (%q) for %q @%d ('%s')",
				err, err, tc.eErr, tc.eErr, tc.t, o, nxtChr)
		} else {
			if o != tc.eOffs {
				t.Errorf("TestParseTokenParam: offset mismatch: %d,"+
					" expected %d for %q",
					o, tc.eOffs, tc.t)
			}
			if !bytes.Equal(param.All.Get(tc.t), []byte(tc.eAll)) {
				t.Errorf("TestParseTokenParam: All mismatch: %q,"+
					" expected %q for %q",
					param.All.Get(tc.t), tc.eAll, tc.t)
			}
			if !bytes.Equal(param.Name.Get(tc.t), []byte(tc.eName)) {
				t.Errorf("TestParseTokenParam: Name mismatch: %q,"+
					" expected %q for %q",
					param.Name.Get(tc.t), tc.eName, tc.t)
			}
			if !bytes.Equal(param.Val.Get(tc.t), []byte(tc.eVal)) {
				t.Errorf("TestParseTokenParam: Val mismatch: %q,"+
					" expected %q for %q",
					param.Val.Get(tc.t), tc.eVal, tc.t)
			}
		}
	}
}

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
		{t: []byte("foo14\r\n"), offs: 0, flags: PTokInputEndF,
			eN: 1, eToks: []string{"foo14"},
			eOffs: 7, eErr: ErrHdrOk},
		{t: []byte("foo15\r"), offs: 0, flags: PTokInputEndF,
			eN: 1, eToks: []string{"foo15"},
			eOffs: 6, eErr: ErrHdrOk},
		{t: []byte("foo16"), offs: 0, flags: PTokInputEndF,
			eN: 1, eToks: []string{"foo16"},
			eOffs: 5, eErr: ErrHdrOk},
		{t: []byte("foo17\r\n"), offs: 5, flags: PTokInputEndF,
			eN: 1, eToks: []string{""},
			eOffs: 7, eErr: ErrHdrEmpty},
		{t: []byte("foo18, bar\r\n"), offs: 0,
			flags: PTokCommaSepF | PTokInputEndF,
			eN:    2, eToks: []string{"foo18", "bar"},
			eOffs: 12, eErr: ErrHdrOk},
		{t: []byte("foo19, bar\r"), offs: 0,
			flags: PTokCommaSepF | PTokInputEndF,
			eN:    2, eToks: []string{"foo19", "bar"},
			eOffs: 11, eErr: ErrHdrOk},
		{t: []byte("foo20, bar"), offs: 0,
			flags: PTokCommaSepF | PTokInputEndF,
			eN:    2, eToks: []string{"foo20", "bar"},
			eOffs: 10, eErr: ErrHdrOk},
		{t: []byte("foo21, bar "), offs: 0,
			flags: PTokCommaSepF | PTokInputEndF,
			eN:    2, eToks: []string{"foo21", "bar"},
			eOffs: 11, eErr: ErrHdrOk},
		{t: []byte("foo22, bar 	\r\n"), offs: 0,
			flags: PTokCommaSepF | PTokInputEndF,
			eN:    2, eToks: []string{"foo22", "bar"},
			eOffs: 14, eErr: ErrHdrOk},
		{t: []byte("foo23, bar , 	\r\n baz \r\n"), offs: 0,
			flags: PTokCommaSepF | PTokInputEndF,
			eN:    3, eToks: []string{"foo23", "bar", "baz"},
			eOffs: 23, eErr: ErrHdrOk},
		{t: []byte("foo24, bar	\r\n , baz \r\n"), offs: 0,
			flags: PTokCommaSepF | PTokInputEndF,
			eN:    3, eToks: []string{"foo24", "bar", "baz"},
			eOffs: 22, eErr: ErrHdrOk},
		{t: []byte("foo25, bar	\r\n , baz \r\nX"), offs: 0,
			flags: PTokCommaSepF,
			eN:    3, eToks: []string{"foo25", "bar", "baz"},
			eOffs: 22, eErr: ErrHdrOk},
		{t: []byte("foo26	 bar	\r\n  baz \r\nX"), offs: 0,
			flags: PTokSpSepF,
			eN:    3, eToks: []string{"foo26", "bar", "baz"},
			eOffs: 21, eErr: ErrHdrOk},
		{t: []byte("foo27 bar	, \r\n  baz \r\n"), offs: 0,
			flags: PTokCommaSepF | PTokSpSepF | PTokInputEndF,
			eN:    3, eToks: []string{"foo27", "bar", "baz"},
			eOffs: 22, eErr: ErrHdrOk},
		{t: []byte("foo28 bar	, \r\n  baz \r\n"), offs: 0,
			flags: PTokCommaSepF | PTokSpSepF,
			eN:    3, eToks: []string{"foo28", "bar", "baz"},
			eOffs: 19, eErr: ErrHdrMoreBytes},
		{t: []byte("foo29 ; p1=v1;p2 = v2 ; p3=v3, bar;p4;p5=v5\r\n\r\n"),
			offs: 0, flags: PTokCommaSepF,
			eN: 2, eToks: []string{"foo29", "bar"},
			eOffs: 6, eErr: ErrHdrBadChar}, // err: params (';') not allowed
		{t: []byte("foo30 ; p1=v1;p2 = v2 ; p3=v3, bar;p4;p5=v5\r\nX"),
			offs: 0, flags: PTokCommaSepF | PTokAllowParamsF,
			eN: 2, eToks: []string{"foo30", "bar"},
			eOffs: 45, eErr: ErrHdrOk}, // ok: flags allow ',' & ';'
		{t: []byte("foo31 ; p1=v1;\r\n p2 = v2 ; p3=v3, bar;p4;p5=v5\r\nX"),
			offs: 0, flags: PTokCommaSepF | PTokAllowParamsF,
			eN: 2, eToks: []string{"foo31", "bar"},
			eOffs: 48, eErr: ErrHdrOk},
		{t: []byte("foo32;p1=v1;p2 = v2 ;p3=v3, bar;p4;p5=v5\r\n"),
			offs: 0, flags: PTokCommaSepF | PTokAllowParamsF | PTokInputEndF,
			eN: 2, eToks: []string{"foo32", "bar"},
			eOffs: 42, eErr: ErrHdrOk},
		{t: []byte("foo33;p1=v1;p2 = v2 ;p3=v3, bar;p4;p5=v5\r\n"),
			offs:  0,
			flags: PTokCommaSepF | PTokSpSepF | PTokAllowParamsF | PTokInputEndF,
			eN:    2, eToks: []string{"foo33", "bar"},
			eOffs: 42, eErr: ErrHdrOk},
		{t: []byte("foo34;p1=v1;p2 = v2 ;p3=v3  bar;p4;p5=v5\r\n"),
			offs:  0,
			flags: PTokCommaSepF | PTokSpSepF | PTokAllowParamsF | PTokInputEndF,
			eN:    2, eToks: []string{"foo34", "bar"},
			eOffs: 42, eErr: ErrHdrOk},
		{t: []byte("foo35;p1=v1;p2 = v2 ;p3=v3  bar;p4;p5=v5 ,  baz ; p6 = v6\r\n\r\n"),
			offs:  0,
			flags: PTokCommaSepF | PTokSpSepF | PTokAllowParamsF | PTokInputEndF,
			eN:    3, eToks: []string{"foo35", "bar", "baz"},
			eOffs: 59, eErr: ErrHdrOk},
		{t: []byte("foo36;p1=v1;p2 = v2 ;p3=v3  bar;p4;p5=v5    baz ; p6 = v6\r\n\r\n"),
			offs:  0,
			flags: PTokCommaSepF | PTokSpSepF | PTokAllowParamsF,
			eN:    3, eToks: []string{"foo36", "bar", "baz"},
			eOffs: 59, eErr: ErrHdrOk},
		{t: []byte("foo37.1;p1.0=v1;p2-0 = v2 ;p3-0.3=v3  bar-2.2;p4;p5=v5    baz ; p6 = v6\r\n\r\n"),
			offs:  0,
			flags: PTokCommaSepF | PTokSpSepF | PTokAllowParamsF,
			eN:    3, eToks: []string{"foo37.1", "bar-2.2", "baz"},
			eOffs: 73, eErr: ErrHdrOk},
	}

	for _, tc := range tests {
		// fix wild card values
		if tc.eOffs <= 0 {
			// assume \r\n + extra char
			tc.eOffs = len(tc.t) - 1
		}

		var tok PToken
		var err ErrorHdr
		var nxtChr string
		n := 0
		o := tc.offs
		for {
			tok.Reset()
			o, err = ParseTokenLst(tc.t, o, &tok, tc.flags)
			if o == len(tc.t) {
				nxtChr = "EOF" // place holder for end of input
			} else if o > len(tc.t) {
				nxtChr = "ERR_OVERFLOW" // place holder for out of buffer
			} else {
				nxtChr = string(tc.t[o])
			}
			/*
				t.Logf("test %q, parsed token %d, offset %d ('%s') => err %q"+
					" state %d", tc.t, n, o, nxtChr, err, tok.state)
			*/
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
					" expected %d (%q) internal state %d",
					n, tc.eN, tc.t, tok.state)
				break
			}
			if err != ErrHdrMoreValues {
				break
			}
			/*
				t.Logf("test %q, next token %d, offset %d ('%s') err %q",
					tc.t, n, o, nxtChr, err)
			*/
		}
		if err != tc.eErr {
			t.Errorf("TestParseTokLst: error code mismatch: %d (%q),"+
				" expected %d (%q) for %q @%d ('%s') (%d/%d tokens found)"+
				" internal state  %d",
				err, err, tc.eErr, tc.eErr, tc.t, o, nxtChr, n, tc.eN,
				tok.state)
		} else if o != tc.eOffs {
			t.Errorf("TestParseTokLst: offset mismatch: %d,"+
				" expected %d for %q (%d/%d tokens found)"+
				" internal state  %d",
				o, tc.eOffs, tc.t, n, tc.eN, tok.state)
		}
	}
}

func TestTokParamN(t *testing.T) {
	type testCase struct {
		t     []byte   // test string (contains params)
		offs  int      // params offset in t
		no    int      // params total number
		flags uint     // parsing flags
		tN    int      // test param n
		eAll  string   // expected trimmed param
		eName string   // expected trimmed param name
		eVal  string   // expected trimmed param value
		eErr  ErrorHdr // expected error
	}

	tests := [...]testCase{
		{t: []byte("t1;a1=v1;a2=v2 ;a3=v3; a4=v4	; 	a5=v5"),
			offs: 3, no: 5, flags: 0, tN: 0,
			eAll: "a1=v1", eName: "a1", eVal: "v1", eErr: ErrHdrOk},
		{t: []byte("t2;a1=v1;a2=v2 ;a3=v3; a4=v4	; 	a5=v5"),
			offs: 3, no: 5, flags: 0, tN: 1,
			eAll: "a2=v2", eName: "a2", eVal: "v2", eErr: ErrHdrOk},
		{t: []byte("t3;a1=v1;a2=v2 ;a3=v3; a4=v4	; 	a5=v5"),
			offs: 3, no: 5, flags: 0, tN: 2,
			eAll: "a3=v3", eName: "a3", eVal: "v3", eErr: ErrHdrOk},
		{t: []byte("t4;a1=v1;a2=v2 ;a3=v3; a4=v4	; 	a5=v5"),
			offs: 3, no: 5, flags: 0, tN: 3,
			eAll: "a4=v4", eName: "a4", eVal: "v4", eErr: ErrHdrOk},
		{t: []byte("t5;a1=v1;a2=v2 ;a3=v3; a4=v4	; 	a5=v5"),
			offs: 3, no: 5, flags: 0, tN: 5,
			eAll: "", eName: "", eVal: "", eErr: ErrHdrEmpty},
		{t: []byte("t6;a1 	= v1,a2=v2 ;a3=v3; a4=v4	; 	a5=v5"),
			offs: 3, no: 5, flags: PTokCommaSepF, tN: 0,
			eAll: "a1 	= v1", eName: "a1", eVal: "v1", eErr: ErrHdrOk},
		{t: []byte("t6;a1 	= v1,a2=v2 ;a3=v3; a4=v4	; 	a5=v5"),
			offs: 3, no: 5, flags: PTokCommaSepF, tN: 1,
			eAll: "", eName: "", eVal: "", eErr: ErrHdrEmpty},
		{t: []byte("t6;a1 	= v1 	a2=v2 ;a3=v3; a4=v4	; 	a5=v5"),
			offs: 3, no: 5, flags: PTokSpSepF | PTokCommaSepF, tN: 2,
			eAll: "", eName: "", eVal: "", eErr: ErrHdrEmpty},
	}
	for _, tc := range tests {
		var tok PToken
		tok.V.Set(0, len(tc.t))
		tok.Params.Set(tc.offs, len(tc.t))
		tok.ParamsNo = uint(tc.no)

		param, err := tok.Param(tc.t, tc.tN, tc.flags)

		if err != tc.eErr {
			t.Errorf("TestTokParamN: error code mismatch: %d (%q),"+
				" expected %d (%q) for %q",
				err, err, tc.eErr, tc.eErr, tc.t)
		} else {
			if !bytes.Equal(param.All.Get(tc.t), []byte(tc.eAll)) {
				t.Errorf("TestTokParamN: All mismatch: %q,"+
					" expected %q for %q",
					param.All.Get(tc.t), tc.eAll, tc.t)
			}
			if !bytes.Equal(param.Name.Get(tc.t), []byte(tc.eName)) {
				t.Errorf("TestTokenParamN: Name mismatch: %q,"+
					" expected %q for %q",
					param.Name.Get(tc.t), tc.eName, tc.t)
			}
			if !bytes.Equal(param.Val.Get(tc.t), []byte(tc.eVal)) {
				t.Errorf("TestTokParamN: Val mismatch: %q,"+
					" expected %q for %q",
					param.Val.Get(tc.t), tc.eVal, tc.t)
			}
		}
	}
}
