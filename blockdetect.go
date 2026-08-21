package qkdpostproc

import (
	"runtime"
	"sync"
)

// step 2 detects errors only, never corrects a bit - an extended
// (SECDED) Hamming syndrome catches every 1/2/3-bit error per block

// hammingSyndrome computes the (r+1)-bit syndrome: r Hamming parity
// bits + 1 overall parity bit, O(r*n) per block, called in parallel
func hammingSyndrome(dataBits []byte) []byte {
	m := len(dataBits)
	if m == 0 {
		return nil
	}
	r := 0
	for (1 << uint(r)) < m+r+1 {
		r++
	}
	n := m + r
	isPow2 := func(x int) bool { return x != 0 && x&(x-1) == 0 }

	codeword := make([]byte, n+1) // 1-indexed conceptual codeword
	di := 0
	for i := 1; i <= n; i++ {
		if isPow2(i) {
			continue
		}
		codeword[i] = dataBits[di]
		di++
	}

	parity := make([]byte, r+1)
	for j := 0; j < r; j++ {
		p := 1 << uint(j)
		var x byte
		for i := 1; i <= n; i++ {
			if i&p != 0 && !isPow2(i) {
				x ^= codeword[i]
			}
		}
		parity[j] = x
	}
	var overall byte
	for _, b := range dataBits {
		overall ^= b
	}
	parity[r] = overall
	return parity
}

func syndromesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// BlockDetectResult summarizes the outcome of running step 2 over the
// full sifted key
type BlockDetectResult struct {
	SurvivingAlice []byte // concatenated bits from blocks with matching syndromes
	SurvivingBob   []byte
	DiscardedAlice []byte // concatenated bits from blocks flagged as errored (for QBER use)
	DiscardedBob   []byte
	BlocksTotal    int
	BlocksKept     int
	BlocksDropped  int
	LeakedBits     int // classical-channel bits spent announcing syndromes
}

type blockOutcome struct {
	kept       bool
	leakedBits int
}

// BlockErrorDetect splits the sifted key into blocks and drops (never
// flips) any block whose Alice/Bob syndromes disagree, in parallel
func BlockErrorDetect(alice, bob []byte, blockSize int) BlockDetectResult {
	if blockSize <= 0 {
		panic("blockSize must be > 0")
	}
	n := len(alice)
	numBlocks := (n + blockSize - 1) / blockSize
	outcomes := make([]blockOutcome, numBlocks)

	workers := runtime.GOMAXPROCS(0)
	if workers > numBlocks {
		workers = numBlocks
	}
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	chunk := (numBlocks + workers - 1) / workers
	for w := 0; w < workers; w++ {
		start := w * chunk
		end := start + chunk
		if end > numBlocks {
			end = numBlocks
		}
		if start >= end {
			continue
		}
		wg.Add(1)
		go func(startBlk, endBlk int) {
			defer wg.Done()
			for bi := startBlk; bi < endBlk; bi++ {
				bs := bi * blockSize
				be := bs + blockSize
				if be > n {
					be = n
				}
				aSynd := hammingSyndrome(alice[bs:be])
				bSynd := hammingSyndrome(bob[bs:be])
				outcomes[bi] = blockOutcome{
					kept:       syndromesEqual(aSynd, bSynd),
					leakedBits: len(aSynd),
				}
			}
		}(start, end)
	}
	wg.Wait()

	var res BlockDetectResult
	res.BlocksTotal = numBlocks

	var survivingLen, discardedLen int
	for bi := 0; bi < numBlocks; bi++ {
		bs := bi * blockSize
		be := bs + blockSize
		if be > n {
			be = n
		}
		blen := be - bs
		if outcomes[bi].kept {
			survivingLen += blen
		} else {
			discardedLen += blen
		}
		res.LeakedBits += outcomes[bi].leakedBits
	}
	res.SurvivingAlice = make([]byte, 0, survivingLen)
	res.SurvivingBob = make([]byte, 0, survivingLen)
	res.DiscardedAlice = make([]byte, 0, discardedLen)
	res.DiscardedBob = make([]byte, 0, discardedLen)

	for bi := 0; bi < numBlocks; bi++ {
		bs := bi * blockSize
		be := bs + blockSize
		if be > n {
			be = n
		}
		if outcomes[bi].kept {
			res.BlocksKept++
			res.SurvivingAlice = append(res.SurvivingAlice, alice[bs:be]...)
			res.SurvivingBob = append(res.SurvivingBob, bob[bs:be]...)
		} else {
			res.BlocksDropped++
			res.DiscardedAlice = append(res.DiscardedAlice, alice[bs:be]...)
			res.DiscardedBob = append(res.DiscardedBob, bob[bs:be]...)
		}
	}
	return res
}
