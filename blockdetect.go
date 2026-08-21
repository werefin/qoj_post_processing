package qkdpostproc

import (
	"runtime"
	"sync"
)

// step 2 detects errors only, never corrects a bit
// extended (SECDED) Hamming syndrome catches every 1/2/3-bit error per block

// hammingRN returns the parity-bit count r and codeword length n = m+r for
// an m-bit data block - lets callers size scratch buffers once per worker
func hammingRN(m int) (r, n int) {
	for (1 << uint(r)) < m+r+1 {
		r++
	}
	return r, m + r
}

// hammingSyndrome computes the (r+1)-bit syndrome: r Hamming parity
// bits + 1 overall parity bit, O(r*n) per block
func hammingSyndrome(dataBits []byte) []byte {
	m := len(dataBits)
	if m == 0 {
		return nil
	}
	r, n := hammingRN(m)
	return hammingSyndromeInto(dataBits, make([]byte, n+1), make([]byte, r+1))
}

// hammingSyndromeInto is hammingSyndrome but writes into caller-supplied
// scratch buffers (sized via hammingRN) instead of allocating each call
func hammingSyndromeInto(dataBits []byte, codeword, parity []byte) []byte {
	m := len(dataBits)
	if m == 0 {
		return nil
	}
	r, n := hammingRN(m)
	isPow2 := func(x int) bool { return x != 0 && x&(x-1) == 0 }

	cw := codeword[:n+1] // 1-indexed conceptual codeword
	di := 0
	for i := 1; i <= n; i++ {
		if isPow2(i) {
			continue
		}
		cw[i] = dataBits[di]
		di++
	}

	pr := parity[:r+1]
	for j := 0; j < r; j++ {
		p := 1 << uint(j)
		var x byte
		for i := 1; i <= n; i++ {
			if i&p != 0 && !isPow2(i) {
				x ^= cw[i]
			}
		}
		pr[j] = x
	}
	var overall byte
	for _, b := range dataBits {
		overall ^= b
	}
	pr[r] = overall
	return pr
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
	LeakedBits     int // classical-channel bits spent announcing syndromes of surviving blocks
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

	// blocks are at most blockSize wide (the last one may be shorter), so
	// buffers sized for a full block cover every block a worker sees
	rMax, nMax := hammingRN(blockSize)

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
			codeword := make([]byte, nMax+1)
			aParity := make([]byte, rMax+1)
			bParity := make([]byte, rMax+1)
			for bi := startBlk; bi < endBlk; bi++ {
				bs := bi * blockSize
				be := bs + blockSize
				if be > n {
					be = n
				}
				aSynd := hammingSyndromeInto(alice[bs:be], codeword, aParity)
				bSynd := hammingSyndromeInto(bob[bs:be], codeword, bParity)
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
			res.LeakedBits += outcomes[bi].leakedBits
		} else {
			discardedLen += blen
		}
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
