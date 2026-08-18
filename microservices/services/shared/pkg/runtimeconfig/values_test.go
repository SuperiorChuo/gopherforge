package runtimeconfig

import "testing"

func TestIntSettingAcceptsWholeNumbersAndRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int
		ok   bool
	}{
		{name: "int", in: int(3), want: 3, ok: true},
		{name: "uint64", in: uint64(4), want: 4, ok: true},
		{name: "whole float", in: float64(5), want: 5, ok: true},
		{name: "fraction", in: 1.5, ok: false},
		{name: "string", in: "5", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := IntSetting(tt.in)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("IntSetting(%v) = (%d, %v), want (%d, %v)", tt.in, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestSettingBoundsUseFallbacks(t *testing.T) {
	value := map[string]any{"zero": 0, "positive": 9, "negative": -1}
	if got := NonNegativeSetting(value, "zero", 7); got != 0 {
		t.Fatalf("NonNegativeSetting zero = %d, want 0", got)
	}
	if got := PositiveSetting(value, "negative", 7); got != 7 {
		t.Fatalf("PositiveSetting negative = %d, want 7", got)
	}
	if got := PositiveSetting(value, "positive", 7); got != 9 {
		t.Fatalf("PositiveSetting positive = %d, want 9", got)
	}
	if got := PositiveOrDefault(0, 7); got != 7 {
		t.Fatalf("PositiveOrDefault zero = %d, want 7", got)
	}
}
