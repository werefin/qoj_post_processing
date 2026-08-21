package qkdpostproc

// Config holds the tunable parameters of the post-processing chain
type Config struct {
	CoincidenceWindowPS int64 // step 1: max |dt| between Alice/Bob clicks
	BlockSize           int   // step 2: Hamming block size, in data bits
	ChunkSize           int   // step 3: CRC chunk size, in bits
}

// Result bundles the outcome of every stage, so callers can inspect
// intermediate statistics (yields, leakage, QBER) as well as the final key
type Result struct {
	SiftedBits int

	BlockDetect BlockDetectResult
	CRCVerify   CRCVerifyResult
	QBER        QBERResult

	LeakedBits int // total classical-channel bits spent on syndromes + CRCs

	FinalKeyBits  []byte // 0/1 per byte
	FinalKeyBytes []byte // packed, MSB-first
	Seed          []byte // Toeplitz seed used (must be agreed/published too)
}

// Run executes the full chain - sifting, detection, CRC, QBER, and
// privacy amplification - over one batch of raw detection events
func Run(aliceEvents, bobEvents []DetectionEvent, cfg Config) (Result, error) {
	var res Result

	sifted := Sift(aliceEvents, bobEvents, cfg.CoincidenceWindowPS)
	res.SiftedBits = len(sifted)

	aliceBits := make([]byte, len(sifted))
	bobBits := make([]byte, len(sifted))
	for i, s := range sifted {
		aliceBits[i] = s.AliceBit
		bobBits[i] = s.BobBit
	}

	res.BlockDetect = BlockErrorDetect(aliceBits, bobBits, cfg.BlockSize)
	res.CRCVerify = CRCVerify(res.BlockDetect.SurvivingAlice, res.BlockDetect.SurvivingBob, cfg.ChunkSize)
	res.QBER = CalculateQBER(
		res.BlockDetect.DiscardedAlice, res.BlockDetect.DiscardedBob,
		res.CRCVerify.DiscardedAlice, res.CRCVerify.DiscardedBob,
	)
	res.LeakedBits = res.BlockDetect.LeakedBits + res.CRCVerify.LeakedBits

	key, seed, err := PrivacyAmplify(res.CRCVerify.SurvivingAlice, res.QBER.QBER, res.LeakedBits)
	if err != nil {
		return res, err
	}
	res.FinalKeyBits = key
	res.FinalKeyBytes = packToBytes(key)
	res.Seed = seed
	return res, nil
}
