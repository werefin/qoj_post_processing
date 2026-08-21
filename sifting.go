package qkdpostproc

import (
	"cmp"
	"slices"
)

// SiftedBit is one surviving, basis-matched, unambiguous coincidence
type SiftedBit struct {
	AliceBit byte
	BobBit   byte
	AliceIdx int // index into the original Alice event slice
	BobIdx   int // index into the original Bob event slice
}

// Sift matches Alice/Bob clicks within windowPS in the same basis
// ambiguous or multi-photon events are dropped on both sides, per spec
func Sift(aliceEvents, bobEvents []DetectionEvent, windowPS int64) []SiftedBit {
	aliceOrder := sortedIndices(aliceEvents)
	bobOrder := sortedIndices(bobEvents)

	bobLo := 0
	// bPos is a dense index into bobOrder, so a slice beats a map here
	// no hashing, no bucket probe, just an array read
	bobCandidateCount := make([]int32, len(bobOrder))

	type match struct{ aPos, bPos int32 }
	matches := make([]match, 0, len(aliceOrder))

	for aPos, aIdx := range aliceOrder {
		aEv := aliceEvents[aIdx]
		if aEv.MultiPhoton {
			continue
		}
		for bobLo < len(bobOrder) && bobEvents[bobOrder[bobLo]].TimestampPS < aEv.TimestampPS-windowPS {
			bobLo++
		}
		count := 0
		lastBPos := int32(-1)
		for bPos := bobLo; bPos < len(bobOrder); bPos++ {
			bIdx := bobOrder[bPos]
			bEv := bobEvents[bIdx]
			if bEv.TimestampPS > aEv.TimestampPS+windowPS {
				break
			}
			if bEv.MultiPhoton || bEv.Basis != aEv.Basis {
				continue
			}
			count++
			lastBPos = int32(bPos)
		}
		if count == 1 {
			matches = append(matches, match{int32(aPos), lastBPos})
			bobCandidateCount[lastBPos]++
		}
	}

	sifted := make([]SiftedBit, 0, len(matches))
	for _, m := range matches {
		if bobCandidateCount[m.bPos] != 1 {
			continue // bob side was ambiguous too, drop
		}
		aIdx := aliceOrder[m.aPos]
		bIdx := bobOrder[m.bPos]
		sifted = append(sifted, SiftedBit{
			AliceBit: aliceEvents[aIdx].Bit,
			BobBit:   bobEvents[bIdx].Bit,
			AliceIdx: int(aIdx),
			BobIdx:   int(bIdx),
		})
	}
	return sifted
}

type tsIdx struct {
	ts  int64
	idx int32
}

// sortedIndices sorts contiguous {timestamp,idx} pairs directly, not an
// index array through sort.Slice's reflect-based closure comparator
func sortedIndices(events []DetectionEvent) []int32 {
	pairs := make([]tsIdx, len(events))
	for i := range events {
		pairs[i] = tsIdx{events[i].TimestampPS, int32(i)}
	}
	slices.SortFunc(pairs, func(a, b tsIdx) int {
		return cmp.Compare(a.ts, b.ts)
	})
	idx := make([]int32, len(pairs))
	for i, p := range pairs {
		idx[i] = p.idx
	}
	return idx
}
