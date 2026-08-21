package qkdpostproc

import (
	"fmt"
	"math/bits"
	"runtime"
	"sync"
)

// step 2 detects errors only, never corrects a bit - each side computes
// p parity bits per m-bit block as P = M*G^T and drops mismatches

// GeneratorMatrix is the p x m parity matrix G: row i computes parity
// bit i as the XOR of the message bits it selects
type GeneratorMatrix [][]byte

// BlockSize returns m, the number of message bits G expects per block
func (g GeneratorMatrix) BlockSize() int { return len(g[0]) }

// ParityBits returns p, the number of parity bits G produces per block
func (g GeneratorMatrix) ParityBits() int { return len(g) }

// GeneratorMatrixEx1/Ex2/Ex3 are three example matrices with different
// code-rate/detection-rate tradeoffs (m=4/m=4/m=6, p=3 each)
var (
	// GeneratorMatrixEx1: code rate 4/7, detects 100% of 1/2-bit and 75%
	// of 3-bit errors, 100% of 4-bit errors
	GeneratorMatrixEx1 = GeneratorMatrix{
		{1, 0, 1, 0},
		{1, 1, 0, 1},
		{0, 0, 1, 1},
	}
	// GeneratorMatrixEx2: same shape as Ex1, trades 4-bit detection (0%)
	// for 100% 3-bit detection
	GeneratorMatrixEx2 = GeneratorMatrix{
		{1, 1, 1, 1},
		{1, 1, 1, 0},
		{0, 1, 1, 0},
	}
	// GeneratorMatrixEx3: code rate 2/3, 88.9% of all bit errors detected
	// overall
	GeneratorMatrixEx3 = GeneratorMatrix{
		{0, 0, 0, 1, 1, 1},
		{1, 1, 0, 1, 0, 0},
		{1, 0, 1, 1, 1, 0},
	}
)

// CascadeGeneratorMatrices stacks g1's rows on g2's, catching collisions
// unique to g1 alone - needs equal block sizes and p1+p2 < m
func CascadeGeneratorMatrices(g1, g2 GeneratorMatrix) (GeneratorMatrix, error) {
	m := g1.BlockSize()
	if g2.BlockSize() != m {
		return nil, fmt.Errorf("cascade: g1 block size %d != g2 block size %d", m, g2.BlockSize())
	}
	p := g1.ParityBits() + g2.ParityBits()
	if p >= m {
		return nil, fmt.Errorf("cascade: combined parity bits %d >= block size %d, no secret would remain", p, m)
	}
	combined := make(GeneratorMatrix, 0, p)
	combined = append(combined, g1...)
	combined = append(combined, g2...)
	return combined, nil
}

// parityMasks precomputes each row of G as a bit-mask, turning the
// M*G^T parity computation into p AND+POPCNT passes per block
type parityMasks struct {
	words int
	rows  [][]uint64
}

// buildParityMasks derives the bit-masks for one G once, so every block
// reuses them - only the words scratch buffer differs per call
func buildParityMasks(g GeneratorMatrix) parityMasks {
	m := g.BlockSize()
	words := (m + 63) / 64
	rows := make([][]uint64, len(g))
	for i, row := range g {
		mask := make([]uint64, words)
		for j, bit := range row {
			if bit != 0 {
				mask[j>>6] |= 1 << uint(j&63)
			}
		}
		rows[i] = mask
	}
	return parityMasks{words: words, rows: rows}
}

// parityInto packs dataBits into words (zero-padded on both sides for a
// short trailing block, so padding never causes a spurious mismatch)
func (pm *parityMasks) parityInto(dataBits []byte, words []uint64, parity []byte) []byte {
	ws := words[:pm.words]
	for i := range ws {
		ws[i] = 0
	}
	for i, b := range dataBits {
		ws[i>>6] |= uint64(b&1) << uint(i&63)
	}
	pr := parity[:len(pm.rows)]
	for i, mask := range pm.rows {
		var pc int
		for k, w := range ws {
			pc += bits.OnesCount64(w & mask[k])
		}
		pr[i] = byte(pc & 1)
	}
	return pr
}

func parityEqual(a, b []byte) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ComputeBlockParity computes the parity vector for every m-bit block,
// using only this side's own bits --> a peer only ever needs the result
func ComputeBlockParity(bits []byte, g GeneratorMatrix) [][]byte {
	blockSize := g.BlockSize()
	if blockSize <= 0 {
		panic("generator matrix must have at least one column")
	}
	n := len(bits)
	numBlocks := (n + blockSize - 1) / blockSize
	parities := make([][]byte, numBlocks)

	workers := max(min(runtime.GOMAXPROCS(0), numBlocks), 1)

	masks := buildParityMasks(g)
	p := g.ParityBits()

	var wg sync.WaitGroup
	chunk := (numBlocks + workers - 1) / workers
	for w := range workers {
		start := w * chunk
		end := min(start+chunk, numBlocks)
		if start >= end {
			continue
		}
		wg.Add(1)
		go func(startBlk, endBlk int) {
			defer wg.Done()
			words := make([]uint64, masks.words)
			scratch := make([]byte, p)
			for bi := startBlk; bi < endBlk; bi++ {
				bs := bi * blockSize
				be := min(bs+blockSize, n)
				out := masks.parityInto(bits[bs:be], words, scratch)
				pb := make([]byte, p)
				copy(pb, out)
				parities[bi] = pb
			}
		}(start, end)
	}
	wg.Wait()
	return parities
}

// BlockReconcileResult summarizes one side's outcome of comparing its own
// parity against a peer's parity, without ever needing the peer's bits
type BlockReconcileResult struct {
	Surviving     []byte
	Discarded     []byte
	BlocksTotal   int
	BlocksKept    int
	BlocksDropped int
	LeakedBits    int // classical-channel bits spent announcing parity of surviving blocks
}

// ReconcileBlocks drops (never flips) any block whose own parity disagrees
// with the peer's - never needs to know the peer's raw key material
func ReconcileBlocks(ownBits []byte, ownParity, peerParity [][]byte, blockSize int) BlockReconcileResult {
	if len(ownParity) != len(peerParity) {
		panic("ownParity and peerParity must cover the same number of blocks")
	}
	numBlocks := len(ownParity)
	n := len(ownBits)

	var res BlockReconcileResult
	res.BlocksTotal = numBlocks

	kept := make([]bool, numBlocks)
	var survivingLen, discardedLen int
	for bi := range numBlocks {
		bs := bi * blockSize
		be := min(bs+blockSize, n)
		blen := be - bs
		ok := parityEqual(ownParity[bi], peerParity[bi])
		kept[bi] = ok
		if ok {
			survivingLen += blen
			res.LeakedBits += len(ownParity[bi])
		} else {
			discardedLen += blen
		}
	}
	res.Surviving = make([]byte, 0, survivingLen)
	res.Discarded = make([]byte, 0, discardedLen)

	for bi := range numBlocks {
		bs := bi * blockSize
		be := min(bs+blockSize, n)
		if kept[bi] {
			res.BlocksKept++
			res.Surviving = append(res.Surviving, ownBits[bs:be]...)
		} else {
			res.BlocksDropped++
			res.Discarded = append(res.Discarded, ownBits[bs:be]...)
		}
	}
	return res
}

// BlockDetectResult summarizes the outcome of running step 2 over the
// full sifted key
type BlockDetectResult struct {
	SurvivingAlice []byte // concatenated bits from blocks with matching parity
	SurvivingBob   []byte
	DiscardedAlice []byte // concatenated bits from blocks flagged as errored (for QBER use)
	DiscardedBob   []byte
	BlocksTotal    int
	BlocksKept     int
	BlocksDropped  int
	LeakedBits     int // classical-channel bits spent announcing parity of surviving blocks
}

// BlockErrorDetect is a convenience wrapper for simulation and testing
// use ComputeBlockParity + ReconcileBlocks directly to run each side apart
func BlockErrorDetect(alice, bob []byte, g GeneratorMatrix) BlockDetectResult {
	blockSize := g.BlockSize()
	aliceParity := ComputeBlockParity(alice, g)
	bobParity := ComputeBlockParity(bob, g)
	aliceRes := ReconcileBlocks(alice, aliceParity, bobParity, blockSize)
	bobRes := ReconcileBlocks(bob, bobParity, aliceParity, blockSize)
	return BlockDetectResult{
		SurvivingAlice: aliceRes.Surviving,
		SurvivingBob:   bobRes.Surviving,
		DiscardedAlice: aliceRes.Discarded,
		DiscardedBob:   bobRes.Discarded,
		BlocksTotal:    aliceRes.BlocksTotal,
		BlocksKept:     aliceRes.BlocksKept,
		BlocksDropped:  aliceRes.BlocksDropped,
		LeakedBits:     aliceRes.LeakedBits, // same p-bit-per-kept-block count on both sides
	}
}
