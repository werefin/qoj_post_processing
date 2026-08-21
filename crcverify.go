package qkdpostproc

import (
	"hash/crc32"
	"runtime"
	"sync"
)

// CRCVerifyResult summarizes step 3
type CRCVerifyResult struct {
	SurvivingAlice []byte // final error-cleaned bits (Alice side, post block+CRC filtering)
	SurvivingBob   []byte
	DiscardedAlice []byte // bits from chunks whose CRCs disagreed (for QBER use)
	DiscardedBob   []byte
	ChunksTotal    int
	ChunksKept     int
	ChunksDropped  int
	LeakedBits     int // classical-channel bits spent announcing CRCs of surviving chunks (32 bits/chunk)
}

// bitsToBytes packs 0/1 bits (MSB-first) into bytes, zero-padding the
// last byte - both sides pad identically so it never causes a mismatch
func bitsToBytes(bits []byte) []byte {
	return bitsToBytesInto(bits, nil)
}

// bitsToBytesInto is bitsToBytes but reuses dst's backing array instead
// of allocating, so a worker can pack every chunk through one buffer
func bitsToBytesInto(bits []byte, dst []byte) []byte {
	need := (len(bits) + 7) / 8
	if cap(dst) < need {
		dst = make([]byte, need)
	} else {
		dst = dst[:need]
		for i := range dst {
			dst[i] = 0
		}
	}
	for i, b := range bits {
		if b != 0 {
			dst[i/8] |= 1 << uint(7-i%8)
		}
	}
	return dst
}

type chunkOutcome struct {
	kept       bool
	aCRC, bCRC uint32
}

// CRCVerify re-checks step 2's survivors over larger CRC-32 chunks,
// catching >=4-bit errors a Hamming block could miss, in parallel
func CRCVerify(alice, bob []byte, chunkSize int) CRCVerifyResult {
	if chunkSize <= 0 {
		panic("chunkSize must be > 0")
	}
	n := len(alice)
	numChunks := (n + chunkSize - 1) / chunkSize
	outcomes := make([]chunkOutcome, numChunks)

	workers := runtime.GOMAXPROCS(0)
	if workers > numChunks {
		workers = numChunks
	}
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	chunkOfWork := (numChunks + workers - 1) / workers
	for w := 0; w < workers; w++ {
		start := w * chunkOfWork
		end := start + chunkOfWork
		if end > numChunks {
			end = numChunks
		}
		if start >= end {
			continue
		}
		wg.Add(1)
		go func(startC, endC int) {
			defer wg.Done()
			var aBuf, bBuf []byte
			for ci := startC; ci < endC; ci++ {
				cs := ci * chunkSize
				ce := cs + chunkSize
				if ce > n {
					ce = n
				}
				aBuf = bitsToBytesInto(alice[cs:ce], aBuf)
				bBuf = bitsToBytesInto(bob[cs:ce], bBuf)
				aCRC := crc32.ChecksumIEEE(aBuf)
				bCRC := crc32.ChecksumIEEE(bBuf)
				outcomes[ci] = chunkOutcome{kept: aCRC == bCRC, aCRC: aCRC, bCRC: bCRC}
			}
		}(start, end)
	}
	wg.Wait()

	var res CRCVerifyResult
	res.ChunksTotal = numChunks

	var survivingLen, discardedLen int
	for ci := 0; ci < numChunks; ci++ {
		cs := ci * chunkSize
		ce := cs + chunkSize
		if ce > n {
			ce = n
		}
		clen := ce - cs
		if outcomes[ci].kept {
			survivingLen += clen
			res.LeakedBits += 32
		} else {
			discardedLen += clen
		}
	}
	res.SurvivingAlice = make([]byte, 0, survivingLen)
	res.SurvivingBob = make([]byte, 0, survivingLen)
	res.DiscardedAlice = make([]byte, 0, discardedLen)
	res.DiscardedBob = make([]byte, 0, discardedLen)

	for ci := 0; ci < numChunks; ci++ {
		cs := ci * chunkSize
		ce := cs + chunkSize
		if ce > n {
			ce = n
		}
		if outcomes[ci].kept {
			res.ChunksKept++
			res.SurvivingAlice = append(res.SurvivingAlice, alice[cs:ce]...)
			res.SurvivingBob = append(res.SurvivingBob, bob[cs:ce]...)
		} else {
			res.ChunksDropped++
			res.DiscardedAlice = append(res.DiscardedAlice, alice[cs:ce]...)
			res.DiscardedBob = append(res.DiscardedBob, bob[cs:ce]...)
		}
	}
	return res
}
