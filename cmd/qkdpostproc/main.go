// Command qkdpostproc runs the BBM92 post-processing chain end to end
// over simulated click streams, or embed qkd.Run as a library call instead
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"math/rand"
	"runtime"
	"time"

	qkd "qkdpostproc"
)

// simulate generates correlated Alice/Bob click streams with configurable
// error, basis-mismatch, jitter and multi-photon rates - swap for real logs
func simulate(n int, errRate, multiPhotonRate float64, jitterPS int64, seed int64) ([]qkd.DetectionEvent, []qkd.DetectionEvent) {
	r := rand.New(rand.NewSource(seed))
	alice := make([]qkd.DetectionEvent, 0, n)
	bob := make([]qkd.DetectionEvent, 0, n)

	t := int64(0)
	for i := 0; i < n; i++ {
		t += 1000 // nominal 1000 ps spacing between pair-emission attempts
		aBasis := qkd.Basis(r.Intn(2))
		bBasis := qkd.Basis(r.Intn(2))
		aBit := byte(r.Intn(2))
		bBit := aBit
		if aBasis == bBasis && r.Float64() < errRate {
			bBit ^= 1
		} else if aBasis != bBasis {
			bBit = byte(r.Intn(2))
		}

		aMP := r.Float64() < multiPhotonRate
		bMP := r.Float64() < multiPhotonRate

		aJitter := int64(r.Intn(int(2*jitterPS+1))) - jitterPS
		bJitter := int64(r.Intn(int(2*jitterPS+1))) - jitterPS

		alice = append(alice, qkd.DetectionEvent{TimestampPS: t + aJitter, Basis: aBasis, Bit: aBit, MultiPhoton: aMP})
		bob = append(bob, qkd.DetectionEvent{TimestampPS: t + bJitter, Basis: bBasis, Bit: bBit, MultiPhoton: bMP})
	}
	return alice, bob
}

func main() {
	n := flag.Int("n", 20000, "number of raw emission attempts to simulate")
	errRate := flag.Float64("err", 0.03, "intrinsic bit error rate on matched-basis events")
	mpRate := flag.Float64("multiphoton", 0.01, "probability a given click is flagged multi-photon")
	windowPS := flag.Int64("window", 500, "coincidence window in picoseconds")
	jitterPS := flag.Int64("jitter", 300, "simulated detector timing jitter in picoseconds")
	blockSize := flag.Int("block", 24, "step 2 Hamming block size (data bits)")
	chunkSize := flag.Int("chunk", 2048, "step 3 CRC chunk size (bits)")
	seed := flag.Int64("seed", 1, "PRNG seed for the simulation")
	flag.Parse()

	fmt.Printf("qkdpostproc  (GOMAXPROCS=%d)\n\n", runtime.GOMAXPROCS(0))

	aliceEv, bobEv := simulate(*n, *errRate, *mpRate, *jitterPS, *seed)

	cfg := qkd.Config{
		CoincidenceWindowPS: *windowPS,
		BlockSize:           *blockSize,
		ChunkSize:           *chunkSize,
	}

	t0 := time.Now()
	res, err := qkd.Run(aliceEv, bobEv, cfg)
	elapsed := time.Since(t0)
	if err != nil {
		fmt.Println("pipeline error:", err)
		return
	}

	fmt.Printf("Step 1 - Sifting\n")
	fmt.Printf("  raw attempts:      %d\n", *n)
	fmt.Printf("  sifted bits:       %d\n\n", res.SiftedBits)

	bd := res.BlockDetect
	fmt.Printf("Step 2 - Error Detection (block=%d bits)\n", *blockSize)
	fmt.Printf("  blocks total:      %d\n", bd.BlocksTotal)
	fmt.Printf("  blocks kept:       %d\n", bd.BlocksKept)
	fmt.Printf("  blocks dropped:    %d\n", bd.BlocksDropped)
	fmt.Printf("  bits surviving:    %d\n", len(bd.SurvivingAlice))
	fmt.Printf("  bits leaked (syndromes): %d\n\n", bd.LeakedBits)

	cv := res.CRCVerify
	fmt.Printf("Step 3 - Error Verification (chunk=%d bits, CRC-32)\n", *chunkSize)
	fmt.Printf("  chunks total:      %d\n", cv.ChunksTotal)
	fmt.Printf("  chunks kept:       %d\n", cv.ChunksKept)
	fmt.Printf("  chunks dropped:    %d\n", cv.ChunksDropped)
	fmt.Printf("  bits surviving:    %d\n", len(cv.SurvivingAlice))
	fmt.Printf("  bits leaked (CRCs): %d\n\n", cv.LeakedBits)

	fmt.Printf("Step 4 - QBER Calculation\n")
	fmt.Printf("  sample bits (discarded blocks+chunks): %d\n", res.QBER.SampleBits)
	fmt.Printf("  error bits in sample:                  %d\n", res.QBER.ErrorBits)
	fmt.Printf("  QBER estimate:                          %.4f\n\n", res.QBER.QBER)

	residualErr := 0
	for i := range cv.SurvivingAlice {
		if cv.SurvivingAlice[i] != cv.SurvivingBob[i] {
			residualErr++
		}
	}
	fmt.Printf("  (sanity) residual mismatches in cleaned stream: %d / %d\n\n", residualErr, len(cv.SurvivingAlice))

	rate := 0.0
	if len(cv.SurvivingAlice) > 0 {
		rate = float64(len(res.FinalKeyBits)) / float64(len(cv.SurvivingAlice))
	}
	fmt.Printf("Step 5 - Privacy Amplification\n")
	fmt.Printf("  input bits (error-free stream):   %d\n", len(cv.SurvivingAlice))
	fmt.Printf("  classical bits leaked total:      %d\n", res.LeakedBits)
	fmt.Printf("  distilled secret key length:      %d bits\n", len(res.FinalKeyBits))
	fmt.Printf("  compression rate:                 %.4f\n", rate)
	if len(res.FinalKeyBits) > 0 {
		fmt.Printf("  secret key (hex):                  %s\n", hex.EncodeToString(res.FinalKeyBytes))
	} else {
		fmt.Printf("  secret key: (empty - QBER/leakage too high to distil a secure key at this length)\n")
	}
	fmt.Printf("\nwall time: %s\n", elapsed)
}
