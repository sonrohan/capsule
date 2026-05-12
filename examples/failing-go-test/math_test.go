package failinggo

import "testing"

func TestMultiplyByTwo(t *testing.T) {
	if got := Multiply(3, 2); got != 6 {
		t.Fatalf("Multiply(3, 2) = %d, want 6", got)
	}
}

func TestIntentionalFailure(t *testing.T) {
	if got := Multiply(3, 3); got != 8 {
		t.Fatalf("Multiply(3, 3) = %d, want 8", got)
	}
}
