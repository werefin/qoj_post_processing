package qkdpostproc

import (
	"bytes"
	"math/rand"
	"testing"
)

func TestCRCVerifyCatchesMismatch(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	n := 1000
	alice := make([]byte, n)
	for i := range alice {
		alice[i] = byte(r.Intn(2))
	}
	bob := append([]byte(nil), alice...)
	bob[550] ^= 1 // single error inside chunk 2 (chunkSize 256 -> chunk index 2)

	chunkSize := 256
	res := CRCVerify(alice, bob, chunkSize)
	if res.ChunksTotal != 4 {
		t.Fatalf("expected 4 chunks, got %d", res.ChunksTotal)
	}
	if res.ChunksDropped != 1 {
		t.Fatalf("expected exactly 1 dropped chunk, got %d", res.ChunksDropped)
	}
	for i := range res.SurvivingAlice {
		if res.SurvivingAlice[i] != res.SurvivingBob[i] {
			t.Fatalf("surviving CRC-verified stream has a mismatch at %d", i)
		}
	}
}

func TestCRCVerifyIdenticalStreamsAllKept(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	n := 2000
	alice := make([]byte, n)
	for i := range alice {
		alice[i] = byte(r.Intn(2))
	}
	bob := append([]byte(nil), alice...)
	res := CRCVerify(alice, bob, 300)
	if res.ChunksDropped != 0 {
		t.Fatalf("expected 0 dropped chunks for identical streams, got %d", res.ChunksDropped)
	}
	if len(res.SurvivingAlice) != n {
		t.Fatalf("expected all %d bits to survive, got %d", n, len(res.SurvivingAlice))
	}
}

// TestCRCSplitMatchesCombined: ComputeChunkCRC + ReconcileChunks run per
// side give the same result as the combined convenience wrapper
func TestCRCSplitMatchesCombined(t *testing.T) {
	r := rand.New(rand.NewSource(9))
	n := 5001
	chunkSize := 300
	alice := make([]byte, n)
	bob := make([]byte, n)
	for i := range alice {
		b := byte(r.Intn(2))
		alice[i] = b
		bob[i] = b
		if r.Float64() < 0.01 {
			bob[i] ^= 1
		}
	}

	want := CRCVerify(alice, bob, chunkSize)

	aliceCRC := ComputeChunkCRC(alice, chunkSize)
	bobCRC := ComputeChunkCRC(bob, chunkSize)
	aliceRes := ReconcileChunks(alice, aliceCRC, bobCRC, chunkSize)
	bobRes := ReconcileChunks(bob, bobCRC, aliceCRC, chunkSize)

	if !bytes.Equal(aliceRes.Surviving, want.SurvivingAlice) {
		t.Fatal("Alice's split surviving bits diverged from the combined result")
	}
	if !bytes.Equal(bobRes.Surviving, want.SurvivingBob) {
		t.Fatal("Bob's split surviving bits diverged from the combined result")
	}
	if !bytes.Equal(aliceRes.Discarded, want.DiscardedAlice) {
		t.Fatal("Alice's split discarded bits diverged from the combined result")
	}
	if aliceRes.LeakedBits != want.LeakedBits {
		t.Fatalf("expected %d leaked bits, got %d", want.LeakedBits, aliceRes.LeakedBits)
	}
}
