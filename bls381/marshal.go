// Copyright 2020 ConsenSys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by gurvy DO NOT EDIT

package bls381

import (
	"encoding/binary"
	"errors"
	"io"
	"reflect"
	"sync/atomic"

	"github.com/consensys/gurvy/bls381/fp"
	"github.com/consensys/gurvy/bls381/fr"
	"github.com/consensys/gurvy/bls381/internal/fptower"
	"github.com/consensys/gurvy/utils/parallel"
)

// To encode G1Affine and G2Affine points, we mask the most significant bits with these bits to specify without ambiguity
// metadata needed for point (de)compression
// we follow the BLS381 style encoding as specified in ZCash and now IETF
// The most significant bit, when set, indicates that the point is in compressed form. Otherwise, the point is in uncompressed form.
// The second-most significant bit indicates that the point is at infinity. If this bit is set, the remaining bits of the group element's encoding should be set to zero.
// The third-most significant bit is set if (and only if) this point is in compressed form and it is not the point at infinity and its y-coordinate is the lexicographically largest of the two associated with the encoded x-coordinate.
const (
	mMask                 byte = 0b111 << 5
	mUncompressed         byte = 0b000 << 5
	mUncompressedInfinity byte = 0b010 << 5
	mCompressedSmallest   byte = 0b100 << 5
	mCompressedLargest    byte = 0b101 << 5
	mCompressedInfinity   byte = 0b110 << 5
)

// SizeOfGT represents the size in bytes that a GT element need in binary form
const SizeOfGT = fptower.SizeOfGT

// Encoder writes bls381 object values to an output stream
type Encoder struct {
	w   io.Writer
	n   int64 // written bytes
	raw bool  // raw vs compressed encoding
}

// Decoder reads bls381 object values from an inbound stream
type Decoder struct {
	r io.Reader
	n int64 // read bytes
}

// NewDecoder returns a binary decoder supporting curve bls381 objects in both
// compressed and uncompressed (raw) forms
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{r: r}
}

// Decode reads the binary encoding of v from the stream
// type must be *uint64, *fr.Element, *fp.Element, *G1Affine, *G2Affine, *[]G1Affine or *[]G2Affine
func (dec *Decoder) Decode(v interface{}) (err error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() || !rv.Elem().CanSet() {
		return errors.New("bls381 decoder: unsupported type, need pointer")
	}

	// implementation note: code is a bit verbose (abusing code generation), but minimize allocations on the heap
	// TODO double check memory usage and factorize this

	var buf [SizeOfG2AffineUncompressed]byte
	var read int

	switch t := v.(type) {
	case *uint64:
		var r uint64
		r, err = dec.readUint64()
		if err != nil {
			return
		}
		*t = r
		return
	case *fr.Element:
		read, err = io.ReadFull(dec.r, buf[:fr.Limbs*8])
		dec.n += int64(read)
		if err != nil {
			return
		}
		t.SetBytes(buf[:fr.Limbs*8])
		return
	case *fp.Element:
		read, err = io.ReadFull(dec.r, buf[:fp.Limbs*8])
		dec.n += int64(read)
		if err != nil {
			return
		}
		t.SetBytes(buf[:fp.Limbs*8])
		return
	case *G1Affine:
		// we start by reading compressed point size, if metadata tells us it is uncompressed, we read more.
		read, err = io.ReadFull(dec.r, buf[:SizeOfG1AffineCompressed])
		dec.n += int64(read)
		if err != nil {
			return
		}
		nbBytes := SizeOfG1AffineCompressed
		// most significant byte contains metadata
		if !isCompressed(buf[0]) {
			nbBytes = SizeOfG1AffineUncompressed
			// we read more.
			read, err = io.ReadFull(dec.r, buf[SizeOfG1AffineCompressed:SizeOfG1AffineUncompressed])
			dec.n += int64(read)
			if err != nil {
				return
			}
		}
		_, err = t.SetBytes(buf[:nbBytes])
		return
	case *G2Affine:
		// we start by reading compressed point size, if metadata tells us it is uncompressed, we read more.
		read, err = io.ReadFull(dec.r, buf[:SizeOfG2AffineCompressed])
		dec.n += int64(read)
		if err != nil {
			return
		}
		nbBytes := SizeOfG2AffineCompressed
		// most significant byte contains metadata
		if !isCompressed(buf[0]) {
			nbBytes = SizeOfG2AffineUncompressed
			// we read more.
			read, err = io.ReadFull(dec.r, buf[SizeOfG2AffineCompressed:SizeOfG2AffineUncompressed])
			dec.n += int64(read)
			if err != nil {
				return
			}
		}
		_, err = t.SetBytes(buf[:nbBytes])
		return
	case *[]G1Affine:
		var sliceLen uint32
		sliceLen, err = dec.readUint32()
		if err != nil {
			return
		}
		if len(*t) != int(sliceLen) {
			*t = make([]G1Affine, sliceLen)
		}
		compressed := make([]bool, sliceLen)
		for i := 0; i < len(*t); i++ {

			// we start by reading compressed point size, if metadata tells us it is uncompressed, we read more.
			read, err = io.ReadFull(dec.r, buf[:SizeOfG1AffineCompressed])
			dec.n += int64(read)
			if err != nil {
				return
			}
			nbBytes := SizeOfG1AffineCompressed
			// most significant byte contains metadata
			if !isCompressed(buf[0]) {
				nbBytes = SizeOfG1AffineUncompressed
				// we read more.
				read, err = io.ReadFull(dec.r, buf[SizeOfG1AffineCompressed:SizeOfG1AffineUncompressed])
				dec.n += int64(read)
				if err != nil {
					return
				}
				_, err = (*t)[i].SetBytes(buf[:nbBytes])
				if err != nil {
					return
				}
			} else {
				compressed[i] = !((*t)[i].unsafeSetCompressedBytes(buf[:nbBytes]))
			}
		}
		var nbErrs uint64
		parallel.Execute(len(compressed), func(start, end int) {
			for i := start; i < end; i++ {
				if compressed[i] {
					if err := (*t)[i].unsafeComputeY(); err != nil {
						atomic.AddUint64(&nbErrs, 1)
					}
				}
			}
		})
		if nbErrs != 0 {
			return errors.New("point decompression failed")
		}

		return nil
	case *[]G2Affine:
		var sliceLen uint32
		sliceLen, err = dec.readUint32()
		if err != nil {
			return
		}
		if len(*t) != int(sliceLen) {
			*t = make([]G2Affine, sliceLen)
		}
		compressed := make([]bool, sliceLen)
		for i := 0; i < len(*t); i++ {

			// we start by reading compressed point size, if metadata tells us it is uncompressed, we read more.
			read, err = io.ReadFull(dec.r, buf[:SizeOfG2AffineCompressed])
			dec.n += int64(read)
			if err != nil {
				return
			}
			nbBytes := SizeOfG2AffineCompressed
			// most significant byte contains metadata
			if !isCompressed(buf[0]) {
				nbBytes = SizeOfG2AffineUncompressed
				// we read more.
				read, err = io.ReadFull(dec.r, buf[SizeOfG2AffineCompressed:SizeOfG2AffineUncompressed])
				dec.n += int64(read)
				if err != nil {
					return
				}
				_, err = (*t)[i].SetBytes(buf[:nbBytes])
				if err != nil {
					return
				}
			} else {
				compressed[i] = !((*t)[i].unsafeSetCompressedBytes(buf[:nbBytes]))
			}
		}
		var nbErrs uint64
		parallel.Execute(len(compressed), func(start, end int) {
			for i := start; i < end; i++ {
				if compressed[i] {
					if err := (*t)[i].unsafeComputeY(); err != nil {
						atomic.AddUint64(&nbErrs, 1)
					}
				}
			}
		})
		if nbErrs != 0 {
			return errors.New("point decompression failed")
		}

		return nil
	default:
		return errors.New("bls381 encoder: unsupported type")
	}
}

// BytesRead return total bytes read from reader
func (dec *Decoder) BytesRead() int64 {
	return dec.n
}

func (dec *Decoder) readUint64() (r uint64, err error) {
	var read int
	var buf [8]byte
	read, err = io.ReadFull(dec.r, buf[:8])
	dec.n += int64(read)
	if err != nil {
		return
	}
	r = binary.BigEndian.Uint64(buf[:8])
	return
}

func (dec *Decoder) readUint32() (r uint32, err error) {
	var read int
	var buf [4]byte
	read, err = io.ReadFull(dec.r, buf[:4])
	dec.n += int64(read)
	if err != nil {
		return
	}
	r = binary.BigEndian.Uint32(buf[:4])
	return
}

func isCompressed(msb byte) bool {
	mData := msb & mMask
	return !((mData == mUncompressed) || (mData == mUncompressedInfinity))
}

// NewEncoder returns a binary encoder supporting curve bls381 objects
func NewEncoder(w io.Writer, options ...func(*Encoder)) *Encoder {
	// default settings
	enc := &Encoder{
		w:   w,
		n:   0,
		raw: false,
	}

	// handle options
	for _, option := range options {
		option(enc)
	}

	return enc
}

// Encode writes the binary encoding of v to the stream
// type must be uint64, *fr.Element, *fp.Element, *G1Affine, *G2Affine, []G1Affine or []G2Affine
func (enc *Encoder) Encode(v interface{}) (err error) {
	if enc.raw {
		return enc.encodeRaw(v)
	}
	return enc.encode(v)
}

// BytesWritten return total bytes written on writer
func (enc *Encoder) BytesWritten() int64 {
	return enc.n
}

// RawEncoding returns an option to use in NewEncoder(...) which sets raw encoding mode to true
// points will not be compressed using this option
func RawEncoding() func(*Encoder) {
	return func(enc *Encoder) {
		enc.raw = true
	}
}

func (enc *Encoder) encode(v interface{}) (err error) {

	// implementation note: code is a bit verbose (abusing code generation), but minimize allocations on the heap
	// TODO double check memory usage and factorize this

	var written int
	switch t := v.(type) {
	case uint64:
		err = binary.Write(enc.w, binary.BigEndian, t)
		enc.n += 8
		return
	case *fr.Element:
		buf := t.Bytes()
		written, err = enc.w.Write(buf[:])
		enc.n += int64(written)
		return
	case *fp.Element:
		buf := t.Bytes()
		written, err = enc.w.Write(buf[:])
		enc.n += int64(written)
		return
	case *G1Affine:
		buf := t.Bytes()
		written, err = enc.w.Write(buf[:])
		enc.n += int64(written)
		return
	case *G2Affine:
		buf := t.Bytes()
		written, err = enc.w.Write(buf[:])
		enc.n += int64(written)
		return
	case []G1Affine:
		// write slice length
		err = binary.Write(enc.w, binary.BigEndian, uint32(len(t)))
		if err != nil {
			return
		}
		enc.n += 4

		var buf [SizeOfG1AffineCompressed]byte

		for i := 0; i < len(t); i++ {
			buf = t[i].Bytes()
			written, err = enc.w.Write(buf[:])
			enc.n += int64(written)
			if err != nil {
				return
			}
		}
		return nil
	case []G2Affine:
		// write slice length
		err = binary.Write(enc.w, binary.BigEndian, uint32(len(t)))
		if err != nil {
			return
		}
		enc.n += 4

		var buf [SizeOfG2AffineCompressed]byte

		for i := 0; i < len(t); i++ {
			buf = t[i].Bytes()
			written, err = enc.w.Write(buf[:])
			enc.n += int64(written)
			if err != nil {
				return
			}
		}
		return nil
	default:
		return errors.New("<no value> encoder: unsupported type")
	}
}

func (enc *Encoder) encodeRaw(v interface{}) (err error) {

	// implementation note: code is a bit verbose (abusing code generation), but minimize allocations on the heap
	// TODO double check memory usage and factorize this

	var written int
	switch t := v.(type) {
	case uint64:
		err = binary.Write(enc.w, binary.BigEndian, t)
		enc.n += 8
		return
	case *fr.Element:
		buf := t.Bytes()
		written, err = enc.w.Write(buf[:])
		enc.n += int64(written)
		return
	case *fp.Element:
		buf := t.Bytes()
		written, err = enc.w.Write(buf[:])
		enc.n += int64(written)
		return
	case *G1Affine:
		buf := t.RawBytes()
		written, err = enc.w.Write(buf[:])
		enc.n += int64(written)
		return
	case *G2Affine:
		buf := t.RawBytes()
		written, err = enc.w.Write(buf[:])
		enc.n += int64(written)
		return
	case []G1Affine:
		// write slice length
		err = binary.Write(enc.w, binary.BigEndian, uint32(len(t)))
		if err != nil {
			return
		}
		enc.n += 4

		var buf [SizeOfG1AffineUncompressed]byte

		for i := 0; i < len(t); i++ {
			buf = t[i].RawBytes()
			written, err = enc.w.Write(buf[:])
			enc.n += int64(written)
			if err != nil {
				return
			}
		}
		return nil
	case []G2Affine:
		// write slice length
		err = binary.Write(enc.w, binary.BigEndian, uint32(len(t)))
		if err != nil {
			return
		}
		enc.n += 4

		var buf [SizeOfG2AffineUncompressed]byte

		for i := 0; i < len(t); i++ {
			buf = t[i].RawBytes()
			written, err = enc.w.Write(buf[:])
			enc.n += int64(written)
			if err != nil {
				return
			}
		}
		return nil
	default:
		return errors.New("<no value> encoder: unsupported type")
	}
}
