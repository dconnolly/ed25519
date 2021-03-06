// Copyright (c) 2017 George Tankersley. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build !go1.12

package radix51

import "unsafe"

// madd64 multiples two 64-bit numbers and adds them to a split 128-bit accumulator.
func madd64(lo, hi, a, b uint64) (ol uint64, oh uint64) {
	t1 := (a>>32)*(b&0xFFFFFFFF) + ((a & 0xFFFFFFFF) * (b & 0xFFFFFFFF) >> 32)
	t2 := (a&0xFFFFFFFF)*(b>>32) + (t1 & 0xFFFFFFFF)
	ol = (a * b) + lo
	cmp := ol < lo
	oh = hi + (a>>32)*(b>>32) + t1>>32 + t2>>32 + uint64(*(*byte)(unsafe.Pointer(&cmp)))
	return
}
