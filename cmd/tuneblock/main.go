// Command tuneblock sweeps step 2's Hamming block size over one fixed
// simulated click batch and reports which size gives the highest key rate
package main

import (
	"flag"
	"fmt"
	"math/rand"

	qkd "qkdpostproc"
)

// simulate is the same generator cmd/qkdpostproc uses, duplicated here so
// this tool has no dependency beyond the library itself
func simulate(n int, errRate, multiPhotonRate float64, jitterPS int64, seed int64) ([]qkd.DetectionEvent, []qkd.DetectionEvent) {
	r := rand.New(rand.NewSource(seed))
	alice := make([]qkd.DetectionEvent, 0, n)
	bob := make([]qkd.DetectionEvent, 0, n)

	t := int64(0)
	for i := 0; i < n; i++ {
		t += 1000
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

type row struct {
	blockSize      int
	blocksTotal    int
	blocksDropped  int
	discardFrac    float64
	survivingStep2 int
	finalKeyBits   int
	keyRate        float64 // final key bits per raw attempt
	keysPerBatch   int     // whole keyBits-sized keys this batch yields
}

func main() {
	n := flag.Int("n", 300000, "number of raw emission attempts to simulate")
	errRate := flag.Float64("err", 0.03, "intrinsic bit error rate on matched-basis events")
	mpRate := flag.Float64("multiphoton", 0.01, "probability a given click is flagged multi-photon")
	windowPS := flag.Int64("window", 500, "coincidence window in picoseconds")
	jitterPS := flag.Int64("jitter", 300, "simulated detector timing jitter in picoseconds")
	chunkSize := flag.Int("chunk", 2048, "step 3 CRC chunk size, held fixed during the sweep")
	seed := flag.Int64("seed", 1, "PRNG seed for the simulation")
	from := flag.Int("from", 4, "smallest block size to try")
	to := flag.Int("to", 128, "largest block size to try")
	step := flag.Int("step", 4, "block size increment")
	keyBits := flag.Int("keybits", 256, "key size you'll actually consume, for the keys-per-batch column")
	flag.Parse()

	if *from <= 0 || *to < *from || *step <= 0 {
		fmt.Println("invalid -from/-to/-step")
		return
	}

	// one fixed batch reused for every candidate, so the sweep compares
	// block sizes on identical data rather than fresh noise each time
	aliceEv, bobEv := simulate(*n, *errRate, *mpRate, *jitterPS, *seed)

	var rows []row
	for bs := *from; bs <= *to; bs += *step {
		cfg := qkd.Config{CoincidenceWindowPS: *windowPS, BlockSize: bs, ChunkSize: *chunkSize}
		res, err := qkd.Run(aliceEv, bobEv, cfg)
		if err != nil {
			fmt.Printf("block=%d: error: %v\n", bs, err)
			continue
		}
		bd := res.BlockDetect
		discardFrac := 0.0
		if res.SiftedBits > 0 {
			discardFrac = float64(len(bd.DiscardedAlice)) / float64(res.SiftedBits)
		}
		finalBits := len(res.FinalKeyBits)
		rows = append(rows, row{
			blockSize:      bs,
			blocksTotal:    bd.BlocksTotal,
			blocksDropped:  bd.BlocksDropped,
			discardFrac:    discardFrac,
			survivingStep2: len(bd.SurvivingAlice),
			finalKeyBits:   finalBits,
			keyRate:        float64(finalBits) / float64(*n),
			keysPerBatch:   finalBits / *keyBits,
		})
	}
	if len(rows) == 0 {
		fmt.Println("no candidates produced a result")
		return
	}

	bestRate := 0
	for i, r := range rows {
		if r.keyRate > rows[bestRate].keyRate {
			bestRate = i
		}
	}

	fmt.Printf("block | blocks(total/dropped) | step2 discard%% | key rate (bits/attempt) | %d-bit keys/batch\n", *keyBits)
	for i, r := range rows {
		mark := ""
		if i == bestRate {
			mark = " [best rate]"
		}
		fmt.Printf("%5d | %6d / %-7d | %13.2f%% | %22.5f | %14d%s\n",
			r.blockSize, r.blocksTotal, r.blocksDropped, r.discardFrac*100, r.keyRate, r.keysPerBatch, mark)
	}

	best := rows[bestRate]
	fmt.Printf("\nhighest key rate: block=%d (%.5f bits/attempt, %d %d-bit keys from this batch)\n",
		best.blockSize, best.keyRate, best.keysPerBatch, *keyBits)
}
