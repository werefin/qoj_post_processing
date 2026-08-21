package qkdpostproc

import "testing"

func TestSiftBasic(t *testing.T) {
	alice := []DetectionEvent{
		{TimestampPS: 1000, Basis: 0, Bit: 1},
		{TimestampPS: 2000, Basis: 1, Bit: 0},
		{TimestampPS: 3000, Basis: 0, Bit: 1},
	}
	bob := []DetectionEvent{
		{TimestampPS: 1010, Basis: 0, Bit: 1}, // coincides with alice[0], same basis
		{TimestampPS: 2010, Basis: 0, Bit: 0}, // basis mismatch with alice[1] -> dropped
		{TimestampPS: 3005, Basis: 0, Bit: 0}, // coincides with alice[2], same basis, different bit (that's fine, errors are expected)
	}
	got := Sift(alice, bob, 50)
	if len(got) != 2 {
		t.Fatalf("expected 2 sifted bits, got %d", len(got))
	}
	if got[0].AliceBit != 1 || got[0].BobBit != 1 {
		t.Errorf("unexpected first sifted pair: %+v", got[0])
	}
	if got[1].AliceBit != 1 || got[1].BobBit != 0 {
		t.Errorf("unexpected second sifted pair: %+v", got[1])
	}
}

func TestSiftDropsMultiPhoton(t *testing.T) {
	alice := []DetectionEvent{{TimestampPS: 1000, Basis: 0, Bit: 1, MultiPhoton: true}}
	bob := []DetectionEvent{{TimestampPS: 1000, Basis: 0, Bit: 1}}
	got := Sift(alice, bob, 50)
	if len(got) != 0 {
		t.Fatalf("expected multi-photon event to be dropped, got %d sifted bits", len(got))
	}
}

func TestSiftDropsAmbiguousMultipleCandidates(t *testing.T) {
	alice := []DetectionEvent{{TimestampPS: 1000, Basis: 0, Bit: 1}}
	bob := []DetectionEvent{
		{TimestampPS: 990, Basis: 0, Bit: 1},
		{TimestampPS: 1010, Basis: 0, Bit: 0},
	}
	got := Sift(alice, bob, 50)
	if len(got) != 0 {
		t.Fatalf("expected ambiguous pairing to be dropped, got %d sifted bits", len(got))
	}
}

func TestSiftOutsideWindow(t *testing.T) {
	alice := []DetectionEvent{{TimestampPS: 1000, Basis: 0, Bit: 1}}
	bob := []DetectionEvent{{TimestampPS: 1200, Basis: 0, Bit: 1}}
	got := Sift(alice, bob, 50)
	if len(got) != 0 {
		t.Fatalf("expected event outside coincidence window to be dropped, got %d", len(got))
	}
}
