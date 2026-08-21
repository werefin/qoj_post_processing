package qkdpostproc

import (
	"math/bits"
	"runtime"
	"sync"
)

// step 2 detects errors only, never corrects a bit
// extended (SECDED) Hamming syndrome catches every 1/2/3-bit error per block

// hammingRN returns the parity-bit count r and codeword length n = m+r for
// an m-bit data block --> lets callers size scratch buffers once per worker
func hammingRN(m int) (r, n int) {
	for (1 << uint(r)) < m+r+1 {
		r++
	}
	return r, m + r
}

// hammingMasks precomputes which data bits feed each parity bit for one
// block size, turning each syndrome into r+1 AND+POPCNT passes
type hammingMasks struct {
	r, words int
	perBit   [][]uint64 // r+1 masks over data-bit words; last is overall parity
}

// buildHammingMasks derives the SECDED parity-group membership for an
// m-bit block once, so every block reuses the same precomputed masks
func buildHammingMasks(m int) hammingMasks {
	r, n := hammingRN(m)
	isPow2 := func(x int) bool { return x != 0 && x&(x-1) == 0 }
	words := (m + 63) / 64
	masks := make([][]uint64, r+1)
	for j := range masks {
		masks[j] = make([]uint64, words)
	}
	d := 0
	for i := 1; i <= n; i++ {
		if isPow2(i) {
			continue
		}
		for j := 0; j < r; j++ {
			if i&(1<<uint(j)) != 0 {
				masks[j][d>>6] |= 1 << uint(d&63)
			}
		}
		masks[r][d>>6] |= 1 << uint(d&63) // overall parity: every data bit
		d++
	}
	return hammingMasks{r: r, words: words, perBit: masks}
}

// syndromeInto packs dataBits into the words scratch buffer, then computes
// the r+1 parity bits as AND+POPCNT against the precomputed masks
func (hm *hammingMasks) syndromeInto(dataBits []byte, words []uint64, parity []byte) []byte {
	if len(dataBits) == 0 {
		return nil
	}
	ws := words[:hm.words]
	for i := range ws {
		ws[i] = 0
	}
	// shift by b&1 instead of branching on b!=0: near-random dataBits
	// makes a branch here mispredict constantly --> this was the real cost
	for i, b := range dataBits {
		ws[i>>6] |= uint64(b&1) << uint(i&63)
	}
	pr := parity[:hm.r+1]
	for j := 0; j <= hm.r; j++ {
		mj := hm.perBit[j]
		var pc int
		for k, w := range ws {
			pc += bits.OnesCount64(w & mj[k])
		}
		pr[j] = byte(pc & 1)
	}
	return pr
}

// hammingSyndrome computes the (r+1)-bit syndrome, r Hamming parity bits
// plus 1 overall parity bit --> reference form; BlockErrorDetect reuses masks
func hammingSyndrome(dataBits []byte) []byte {
	if len(dataBits) == 0 {
		return nil
	}
	hm := buildHammingMasks(len(dataBits))
	return hm.syndromeInto(dataBits, make([]uint64, hm.words), make([]byte, hm.r+1))
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

	// masks depend only on block length, so build once and let every
	// block reuse them; only the final (possibly shorter) block differs
	mainMasks := buildHammingMasks(blockSize)
	lastLen := n - (numBlocks-1)*blockSize
	lastMasks := mainMasks
	lastIsDifferent := numBlocks > 0 && lastLen != blockSize
	if lastIsDifferent {
		lastMasks = buildHammingMasks(lastLen)
	}
	rMax := mainMasks.r
	if lastMasks.r > rMax {
		rMax = lastMasks.r
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
			words := make([]uint64, mainMasks.words)
			aParity := make([]byte, rMax+1)
			bParity := make([]byte, rMax+1)
			for bi := startBlk; bi < endBlk; bi++ {
				bs := bi * blockSize
				be := bs + blockSize
				if be > n {
					be = n
				}
				masks := &mainMasks
				if lastIsDifferent && bi == numBlocks-1 {
					masks = &lastMasks
				}
				aSynd := masks.syndromeInto(alice[bs:be], words, aParity)
				bSynd := masks.syndromeInto(bob[bs:be], words, bParity)
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
