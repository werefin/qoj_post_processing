package qkdpostproc

import (
	"math/rand"
	"testing"
)

func TestCalculateQBER(t *testing.T) {
	a1 := []byte{0, 1, 1, 0}
	b1 := []byte{0, 0, 1, 1} // 2 mismatches out of 4
	a2 := []byte{1, 1}
	b2 := []byte{1, 0} // 1 mismatch out of 2
	res := CalculateQBER(a1, b1, a2, b2)
	if res.SampleBits != 6 {
		t.Fatalf("expected 6 sample bits, got %d", res.SampleBits)
	}
	if res.ErrorBits != 3 {
		t.Fatalf("expected 3 error bits, got %d", res.ErrorBits)
	}
	want := 3.0 / 6.0
	if res.QBER != want {
		t.Fatalf("expected QBER %.4f, got %.4f", want, res.QBER)
	}
}

func TestCalculateQBEREmptySample(t *testing.T) {
	res := CalculateQBER(nil, nil, nil, nil)
	if res.QBER != 0 || res.SampleBits != 0 {
		t.Fatalf("expected zero QBER/sample on empty input, got %+v", res)
	}
}

// TestRunEndToEnd checks two invariants: no bit is ever flipped
// (surviving streams match), and the key never exceeds the input length
func TestRunEndToEnd(t *testing.T) {
	r := rand.New(rand.NewSource(55))
	n := 50000
	var alice, bob []DetectionEvent
	tcur := int64(0)
	for i := 0; i < n; i++ {
		tcur += 1000
		aBasis := Basis(r.Intn(2))
		bBasis := Basis(r.Intn(2))
		aBit := byte(r.Intn(2))
		bBit := aBit
		if aBasis == bBasis && r.Float64() < 0.01 {
			bBit ^= 1
		} else if aBasis != bBasis {
			bBit = byte(r.Intn(2))
		}
		alice = append(alice, DetectionEvent{TimestampPS: tcur, Basis: aBasis, Bit: aBit})
		bob = append(bob, DetectionEvent{TimestampPS: tcur, Basis: bBasis, Bit: bBit})
	}

	cfg := Config{CoincidenceWindowPS: 500, BlockSize: 24, ChunkSize: 2048}
	res, err := Run(alice, bob, cfg)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if res.SiftedBits == 0 {
		t.Fatal("expected nonzero sifted bits")
	}
	for i := range res.CRCVerify.SurvivingAlice {
		if res.CRCVerify.SurvivingAlice[i] != res.CRCVerify.SurvivingBob[i] {
			t.Fatalf("error-free stream has a residual mismatch at %d", i)
		}
	}
	if len(res.FinalKeyBits) > len(res.CRCVerify.SurvivingAlice) {
		t.Fatalf("final key (%d bits) longer than error-free input (%d bits)",
			len(res.FinalKeyBits), len(res.CRCVerify.SurvivingAlice))
	}
	if len(res.FinalKeyBits) == 0 {
		t.Fatal("expected a nonzero final key at 1% QBER over 50000 attempts")
	}
}
