package qkdpostproc

// QBERResult holds the outcome of step 4
type QBERResult struct {
	ErrorBits  int
	SampleBits int
	QBER       float64
}

// CalculateQBER estimates QBER directly from the blocks/chunks discarded
// in steps 2-3, comparing them bit-for-bit since they carry no secrecy value
func CalculateQBER(discardedAliceBlocks, discardedBobBlocks, discardedAliceChunks, discardedBobChunks []byte) QBERResult {
	var errBits, total int
	count := func(a, b []byte) {
		n := len(a)
		if len(b) < n {
			n = len(b)
		}
		for i := 0; i < n; i++ {
			total++
			if a[i] != b[i] {
				errBits++
			}
		}
	}
	count(discardedAliceBlocks, discardedBobBlocks)
	count(discardedAliceChunks, discardedBobChunks)

	var q float64
	if total > 0 {
		q = float64(errBits) / float64(total)
	}
	return QBERResult{ErrorBits: errBits, SampleBits: total, QBER: q}
}
