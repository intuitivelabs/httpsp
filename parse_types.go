// Copyright 2021 Intuitive Labs GmbH. All rights reserved.
//
// Use of this source code is governed by a source-available license
// that can be found in the LICENSE.txt file in the root of the source
// tree.

//Package httpsp implements HTTP message statefull parsing.
package httpsp

//OffsT is the type used for offset and length used internally in PField.
type OffsT uint16 // uint16 since max buf & msg size <= 65k

// PField is the type for parsed fields (like host, to body a.s.o.).
// it holds and offset an a length inside a buffer.
type PField struct {
	Offs OffsT
	Len  OffsT
}

// Set sets a PField to point to [start:end).
// end points to the first character after the end of the "string".
func (p *PField) Set(start, end int) {
	p.Offs = OffsT(start)
	p.Len = OffsT(end - start)
	if end < start {
		panic("invalid range")
	}
}

// Reset sets a PField to the empty value.
func (p *PField) Reset() {
	p.Offs = 0
	p.Len = 0
}

// Extend "grows" a PField to a new end offset.
// newEnd points after the end of the "string".
func (p *PField) Extend(newEnd int) {
	p.Len = OffsT(newEnd) - p.Offs
	if newEnd < int(p.Offs) {
		panic("invalid end offset")
	}
}

// Empty returns true if the PField has 0 length.
func (p PField) Empty() bool {
	return p.Len == 0
}

// EndOffs returns the end offset of the PField
//  (points.directly after the end of the "string")
func (p PField) EndOffs() int {
	return int(p.Offs) + int(p.Len)
}

// OffsIn returns true if the offset is inside the PField.
func (p PField) OffsIn(offs int) bool {
	if offs >= int(p.Offs) && offs < p.EndOffs() {
		return true
	}
	return false
}

// Get returns a byte slice inside buf, corresponding to the PField.
// See GetPField() for more information.
func (p PField) Get(buf []byte) []byte {
	return GetPField(buf, p)
}

// GetPField returns a byte slice for the corresponding field f, pointing
// inside buf.
func GetPField(buf []byte, f PField) []byte {
	return buf[f.Offs : f.Offs+f.Len]
}
