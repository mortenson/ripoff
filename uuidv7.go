package ripoff

// Forked from https://github.com/google/uuid

// Original copyright notice is as follows:
// Copyright 2023 Google Inc.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

import (
	"math/rand"

	"github.com/google/uuid"
)

const nanoPerMilli = 1000000
const goReleaseDateNano = 1332918000000 * nanoPerMilli

// NewV7FromReader returns a Version 7 UUID based on a deterministic time.
// it use NewRandomFromReader fill random bits.
// On error, NewV7FromReader returns Nil and an error.
func NewV7FromReader(r *rand.Rand) (uuid.UUID, error) {
	uuid, err := uuid.NewRandomFromReader(r)
	if err != nil {
		return uuid, err
	}

	makeV7(uuid[:], r.Int63()%goReleaseDateNano)
	return uuid, nil
}

// makeV7 fill 48 bits time (uuid[0] - uuid[5]), set version b0111 (uuid[6])
// uuid[8] already has the right version number (Variant is 10)
// see function NewV7 and NewV7FromReader
func makeV7(uuid []byte, timestampOffset int64) {
	/*
		 0                   1                   2                   3
		 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		|                           unix_ts_ms                          |
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		|          unix_ts_ms           |  ver  |  rand_a (12 bit seq)  |
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		|var|                        rand_b                             |
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
		|                            rand_b                             |
		+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	*/
	_ = uuid[15] // bounds check

	t, s := getV7Time(timestampOffset)

	uuid[0] = byte(t >> 40)
	uuid[1] = byte(t >> 32)
	uuid[2] = byte(t >> 24)
	uuid[3] = byte(t >> 16)
	uuid[4] = byte(t >> 8)
	uuid[5] = byte(t)

	uuid[6] = 0x70 | (0x0F & byte(s>>8))
	uuid[7] = byte(s)
}

// getV7Time returns the time in milliseconds and nanoseconds / 256.
func getV7Time(nano int64) (milli, seq int64) {
	milli = nano / nanoPerMilli
	// Sequence number is between 0 and 3906 (nanoPerMilli>>8)
	seq = (nano - milli*nanoPerMilli) >> 8
	return milli, seq
}
