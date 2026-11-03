package auth

import "testing"

func BenchmarkHashPasswordDefault(b *testing.B) {
	p := DefaultParams()
	for b.Loop() {
		_ = hashPassword(p, "correct horse battery staple")
	}
}
