package channel

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsPermanent(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", errors.New("boom"), false},
		{"permanent", Permanent("bad recipient", nil), true},
		{"permanent with cause", Permanent("bad recipient", errors.New("400")), true},
		{"permanent wrapped with %w", fmt.Errorf("send: %w", Permanent("bad recipient", nil)), true},
		{"plain wrapped", fmt.Errorf("send: %w", errors.New("boom")), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsPermanent(tc.err); got != tc.want {
				t.Fatalf("IsPermanent(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestPermanentErrorMessage(t *testing.T) {
	if got := Permanent("bad recipient", nil).Error(); got != "bad recipient" {
		t.Fatalf("Error() = %q, want %q", got, "bad recipient")
	}
	if got := Permanent("bad recipient", errors.New("400")).Error(); got != "bad recipient: 400" {
		t.Fatalf("Error() = %q, want %q", got, "bad recipient: 400")
	}
}
