// Copyright 2022 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

package httpsp

import ()

// ChunkVal contains a parsed "chunk" delimiter
type ChunkVal struct {
	Val         PToken // extension token
	Size        int64  // chunk size
	TrailerHdrs HdrLst // trailer headers if last chunk
	state       uint8  // internal state
}

// Reset  re-initializes the internal parsed token.
func (v *ChunkVal) Reset() {
	v.Val.Reset()
	v.Size = 0
	v.TrailerHdrs.Reset()
	v.state = 0
}

// More returns true if there are more chunks following
func (v *ChunkVal) More() bool {
	return v.Size > 0
}

// ParseChunk parses a chunk "delimiter".
// (see rfc 7230 section 4.1)
// The return values are: a new offset after the parsed value that is either
// the chunk data start (after the chunk CRLF) or a  partial offset that can be
// used to continue parsing if the return error is ErrHdrMoreBytes (not
// enough data to parse the chunk delimiter), the chunk size in bytes on
// success  (not including the final CRLF) and an error.
// Note that to skip over the chunk data one should always use the
// returned offset + chunk_size + 2 (final CRLF), even for the final
// chunk (for consistency: offset after the last-chunk, pointing before the
//  final CRLF)
// It can return ErrHdrMoreBytes if more data is needed (the value is not
// fully contained in buf).
func ParseChunk(buf []byte, offs int, chunk *ChunkVal) (int, int64, ErrorHdr) {
	// parsing token list flags
	const flags = PTokAllowParamsF
	const (
		sCnkParse = iota
		sCnkPTrailer
	)

	var next int
	var err ErrorHdr

	size := int64(-1)
retry:
	switch chunk.state {
	case sCnkParse:
		next, err = ParseTokenLst(buf, offs, &chunk.Val, flags)
		switch err {
		case 0:
			//  chunk size
			if sz, ok := hexToU(chunk.Val.V.Get(buf)); !ok {
				return offs, -1, ErrHdrValNotNumber
				// TODO if size > max  return ErrHdrNumTooBig
			} else {
				size = int64(sz)
				chunk.Size = size // save parsed size
				if size == 0 {
					// set state to parse-last-chunk-trailer
					chunk.state = sCnkPTrailer
					offs = next
					goto retry
				}
			}
		case ErrHdrMoreBytes:
		// do nothing, just for readability
		default:
			chunk.Reset() // some error -> clear the crt tmp state
		}
	case sCnkPTrailer:
		// parse trailer headers
		next, err = ParseHeaders(buf, offs, &chunk.TrailerHdrs, nil)
		if err == ErrHdrEmpty {
			// trailer chunk with no headers => ok
			err = 0
			next -= 2 // return before the final CRLF
		} else if err == 0 {
			next -= 2 // return before the final CRLF
		}
		size = chunk.Size // restore parsed size
		// TODO: some error checking on success: error on disallowed headers?
	}
	return next, size, err
}
