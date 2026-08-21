package qkdpostproc

import (
	"bytes"
	"math/rand"
	"testing"
)

// TestParityKnownVectors checks parityInto against a table of known
// M --> P pairs for GeneratorMatrixEx3, computed independently by hand
func TestParityKnownVectors(t *testing.T) {
	cases := []struct {
		m []byte
		p []byte
	}{
		{[]byte{0, 0, 0, 0, 0, 0}, []byte{0, 0, 0}},
		{[]byte{0, 0, 0, 0, 0, 1}, []byte{1, 0, 0}},
		{[]byte{0, 0, 0, 0, 1, 0}, []byte{1, 0, 1}},
		{[]byte{0, 0, 0, 0, 1, 1}, []byte{0, 0, 1}},
		{[]byte{1, 1, 1, 1, 0, 0}, []byte{1, 1, 1}},
		{[]byte{1, 1, 1, 1, 0, 1}, []byte{0, 1, 1}},
		{[]byte{1, 1, 1, 1, 1, 0}, []byte{0, 1, 0}},
		{[]byte{1, 1, 1, 1, 1, 1}, []byte{1, 1, 0}},
	}
	masks := buildParityMasks(GeneratorMatrixEx3)
	words := make([]uint64, masks.words)
	parity := make([]byte, GeneratorMatrixEx3.ParityBits())
	for _, c := range cases {
		got := masks.parityInto(c.m, words, parity)
		if !parityEqual(got, c.p) {
			t.Fatalf("M=%v: expected P=%v, got %v", c.m, c.p, got)
		}
	}
}

// TestParityCollision checks a known collision under Ex1: two different
// blocks that happen to give identical parity
func TestParityCollision(t *testing.T) {
	masks := buildParityMasks(GeneratorMatrixEx1)
	words := make([]uint64, masks.words)
	p1 := append([]byte(nil), masks.parityInto([]byte{0, 0, 0, 0}, words, make([]byte, 3))...)
	p2 := masks.parityInto([]byte{1, 0, 1, 1}, words, make([]byte, 3))
	want := []byte{0, 0, 0}
	if !parityEqual(p1, want) || !parityEqual(p2, want) {
		t.Fatalf("expected M=(0,0,0,0) and M=(1,0,1,1) to both give P=%v under Ex1, got %v and %v", want, p1, p2)
	}
}

// TestCascadeCatchesEx3Collision: A and B collide under Ex3 alone (same
// parity despite differing by 3 bits), but a small G2 tells them apart
func TestCascadeCatchesEx3Collision(t *testing.T) {
	a := []byte{0, 0, 0, 0, 0, 0}
	b := []byte{0, 0, 1, 0, 1, 1}

	masks := buildParityMasks(GeneratorMatrixEx3)
	words := make([]uint64, masks.words)
	pa := append([]byte(nil), masks.parityInto(a, words, make([]byte, 3))...)
	pb := masks.parityInto(b, words, make([]byte, 3))
	if !parityEqual(pa, pb) {
		t.Fatalf("expected A and B to collide under Ex3 alone, got %v and %v", pa, pb)
	}

	g2 := GeneratorMatrix{
		{1, 0, 0, 0, 0, 0},
		{0, 0, 1, 0, 0, 0},
	}
	cascaded, err := CascadeGeneratorMatrices(GeneratorMatrixEx3, g2)
	if err != nil {
		t.Fatalf("CascadeGeneratorMatrices: %v", err)
	}

	res := BlockErrorDetect(a, b, cascaded)
	if res.BlocksKept != 0 || res.BlocksDropped != 1 {
		t.Fatalf("expected the cascade to catch the collision and drop the block, got kept=%d dropped=%d", res.BlocksKept, res.BlocksDropped)
	}
}

func TestCascadeGeneratorMatricesRejectsMismatchedBlockSize(t *testing.T) {
	_, err := CascadeGeneratorMatrices(GeneratorMatrixEx1, GeneratorMatrixEx3)
	if err == nil {
		t.Fatal("expected an error for mismatched block sizes (m=4 vs m=6)")
	}
}

func TestCascadeGeneratorMatricesRejectsNoRemainingSecret(t *testing.T) {
	_, err := CascadeGeneratorMatrices(GeneratorMatrixEx1, GeneratorMatrixEx2)
	if err == nil {
		t.Fatal("expected an error: Ex1+Ex2 leak 6 parity bits from a 4-bit block")
	}
}

func TestBlockErrorDetectDropsErroredBlocksOnly(t *testing.T) {
	g := GeneratorMatrixEx3
	blockSize := g.BlockSize() // 6, 100% 2-bit-error detection
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
	bob[3*blockSize+2] ^= 1

	res := BlockErrorDetect(alice, bob, g)
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
	if res.LeakedBits != g.ParityBits()*(numBlocks-1) {
		t.Fatalf("expected %d leaked bits, got %d", g.ParityBits()*(numBlocks-1), res.LeakedBits)
	}
}

func TestBlockErrorDetectHandlesShortFinalBlock(t *testing.T) {
	alice := make([]byte, 37) // not a multiple of the m=6 block size
	bob := make([]byte, 37)
	res := BlockErrorDetect(alice, bob, GeneratorMatrixEx3)
	if res.BlocksTotal != 7 { // 6*6=36, plus a 1-bit final block
		t.Fatalf("expected 7 blocks, got %d", res.BlocksTotal)
	}
}

// TestSplitReconciliationMatchesCombined: ComputeBlockParity + Reconcile
// run per side give the same result as the combined convenience wrapper
func TestSplitReconciliationMatchesCombined(t *testing.T) {
	g := GeneratorMatrixEx3
	blockSize := g.BlockSize()
	r := rand.New(rand.NewSource(7))
	n := 6001
	alice := make([]byte, n)
	bob := make([]byte, n)
	for i := range alice {
		b := byte(r.Intn(2))
		alice[i] = b
		bob[i] = b
		if r.Float64() < 0.05 {
			bob[i] ^= 1
		}
	}

	want := BlockErrorDetect(alice, bob, g)

	// Each side only ever touches its own bits plus the parity it
	// received --> never the peer's raw bits
	aliceParity := ComputeBlockParity(alice, g)
	bobParity := ComputeBlockParity(bob, g)
	aliceRes := ReconcileBlocks(alice, aliceParity, bobParity, blockSize)
	bobRes := ReconcileBlocks(bob, bobParity, aliceParity, blockSize)

	if !bytes.Equal(aliceRes.Surviving, want.SurvivingAlice) {
		t.Fatalf("Alice's split surviving bits diverged from the combined result")
	}
	if !bytes.Equal(bobRes.Surviving, want.SurvivingBob) {
		t.Fatalf("Bob's split surviving bits diverged from the combined result")
	}
	if !bytes.Equal(aliceRes.Discarded, want.DiscardedAlice) {
		t.Fatalf("Alice's split discarded bits diverged from the combined result")
	}
	if aliceRes.LeakedBits != want.LeakedBits {
		t.Fatalf("expected %d leaked bits, got %d", want.LeakedBits, aliceRes.LeakedBits)
	}
}
