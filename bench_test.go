package qkdpostproc

import (
	"math/rand"
	"testing"
)

func benchInputs(n, l int) (x, seed []byte) {
	r := rand.New(rand.NewSource(1))
	x = randomBits(r, n)
	seed = randomBits(r, l+n-1)
	return
}

func BenchmarkToeplitzNaive_4kx2k(b *testing.B) {
	x, seed := benchInputs(4096, 2048)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toeplitzHashNaive(x, seed, 2048)
	}
}

func BenchmarkToeplitzFast_4kx2k(b *testing.B) {
	x, seed := benchInputs(4096, 2048)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toeplitzHashFast(x, seed, 2048)
	}
}

func BenchmarkToeplitzFast_100kx50k(b *testing.B) {
	x, seed := benchInputs(100000, 50000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		toeplitzHashFast(x, seed, 50000)
	}
}

// BenchmarkToeplitzImpl_* compares every implementation head-to-head
// toeplitzHashFast always equals the fastest of these on the current CPU
func BenchmarkToeplitzImpl_4kx2k(b *testing.B) {
	x, seed := benchInputs(4096, 2048)
	for name, impl := range toeplitzImpls {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				impl(x, seed, 2048)
			}
		})
	}
}

func BenchmarkToeplitzImpl_100kx50k(b *testing.B) {
	x, seed := benchInputs(100000, 50000)
	for name, impl := range toeplitzImpls {
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				impl(x, seed, 50000)
			}
		})
	}
}

func BenchmarkBlockErrorDetect_1M(b *testing.B) {
	r := rand.New(rand.NewSource(2))
	n := 1_000_000
	alice := randomBits(r, n)
	bob := append([]byte(nil), alice...)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		BlockErrorDetect(alice, bob, 24)
	}
}

func BenchmarkCRCVerify_1M(b *testing.B) {
	r := rand.New(rand.NewSource(2))
	n := 1_000_000
	alice := randomBits(r, n)
	bob := append([]byte(nil), alice...)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CRCVerify(alice, bob, 2048)
	}
}
