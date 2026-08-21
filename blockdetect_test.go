package qkdpostproc

import (
	"math/rand"
	"testing"
)

// TestHammingSyndromeDetectsUpTo3BitErrors: for a block with 1, 2, or 3
// bits flipped the syndrome MUST differ (min-distance-4 code guarantee)
func TestHammingSyndromeDetectsUpTo3BitErrors(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	for trial := 0; trial < 2000; trial++ {
		m := 4 + r.Intn(60) // block sizes 4..63
		orig := make([]byte, m)
		for i := range orig {
			orig[i] = byte(r.Intn(2))
		}
		weight := 1 + r.Intn(3) // 1, 2 or 3 bit errors
		flipped := append([]byte(nil), orig...)
		positions := r.Perm(m)[:weight]
		for _, p := range positions {
			flipped[p] ^= 1
		}
		s1 := hammingSyndrome(orig)
		s2 := hammingSyndrome(flipped)
		if syndromesEqual(s1, s2) {
			t.Fatalf("weight-%d error at positions %v in block size %d went undetected (orig=%v flipped=%v)",
				weight, positions, m, orig, flipped)
		}
	}
}

func TestHammingSyndromeNoErrorMatches(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for trial := 0; trial < 200; trial++ {
		m := 4 + r.Intn(60)
		data := make([]byte, m)
		for i := range data {
			data[i] = byte(r.Intn(2))
		}
		s1 := hammingSyndrome(data)
		s2 := hammingSyndrome(append([]byte(nil), data...))
		if !syndromesEqual(s1, s2) {
			t.Fatalf("identical blocks produced different syndromes")
		}
	}
}

func TestBlockErrorDetectDropsErroredBlocksOnly(t *testing.T) {
	blockSize := 16
	numBlocks := 20
	alice := make([]byte, blockSize*numBlocks)
	bob := make([]byte, blockSize*numBlocks)
	r := rand.New(rand.NewSource(1))
	for i := range alice {
		b := byte(r.Intn(2))
		alice[i] = b
		bob[i] = b
	}
	// Introduce a 2-bit error in block 3 only
	bob[3*blockSize+1] ^= 1
	bob[3*blockSize+5] ^= 1

	res := BlockErrorDetect(alice, bob, blockSize)
	if res.BlocksTotal != numBlocks {
		t.Fatalf("expected %d blocks, got %d", numBlocks, res.BlocksTotal)
	}
	if res.BlocksDropped != 1 {
		t.Fatalf("expected exactly 1 dropped block, got %d", res.BlocksDropped)
	}
	if len(res.SurvivingAlice) != blockSize*(numBlocks-1) {
		t.Fatalf("unexpected surviving length: %d", len(res.SurvivingAlice))
	}
	// Bits are never flipped: surviving Alice/Bob bits must still match
	// exactly, and discarded bits must be exactly the errored block
	for i := range res.SurvivingAlice {
		if res.SurvivingAlice[i] != res.SurvivingBob[i] {
			t.Fatalf("surviving stream has a mismatch at %d - a bit was corrected instead of discarded", i)
		}
	}
	if len(res.DiscardedAlice) != blockSize {
		t.Fatalf("expected discarded block of size %d, got %d", blockSize, len(res.DiscardedAlice))
	}
}

func TestBlockErrorDetectHandlesShortFinalBlock(t *testing.T) {
	alice := make([]byte, 37) // not a multiple of blockSize
	bob := make([]byte, 37)
	res := BlockErrorDetect(alice, bob, 16)
	if res.BlocksTotal != 3 { // 16, 16, 5
		t.Fatalf("expected 3 blocks (16/16/5), got %d", res.BlocksTotal)
	}
}
