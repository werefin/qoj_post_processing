package qkdpostproc

// QBERResult holds the outcome of step 4
type QBERResult struct {
	ErrorBits  int
	SampleBits int
	QBER       float64
}

// CalculateQBER: bad bits in the discarded blocks over all bits of the
// sifted key, faulty and non-faulty blocks combined
func CalculateQBER(discardedAliceBlocks, discardedBobBlocks []byte, siftedBits int) QBERResult {
	errBits := 0
	n := len(discardedAliceBlocks)
	if len(discardedBobBlocks) < n {
		n = len(discardedBobBlocks)
	}
	for i := 0; i < n; i++ {
		if discardedAliceBlocks[i] != discardedBobBlocks[i] {
			errBits++
		}
	}
	var q float64
	if siftedBits > 0 {
		q = float64(errBits) / float64(siftedBits)
	}
	return QBERResult{ErrorBits: errBits, SampleBits: siftedBits, QBER: q}
}
