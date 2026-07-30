package node

import (
	"encoding/json"
	"testing"
)

func TestBTCAmountToSatoshi(t *testing.T) {
	tests := []struct {
		amount json.Number
		want   uint64
		valid  bool
	}{
		{amount: json.Number("0"), want: 0, valid: true},
		{amount: json.Number("0.00000001"), want: 1, valid: true},
		{amount: json.Number("1.23456789"), want: 123456789, valid: true},
		{amount: json.Number("21000000"), want: 2100000000000000, valid: true},
		{amount: json.Number("-1"), valid: false},
		{amount: json.Number("invalid"), valid: false},
	}

	for _, tt := range tests {
		got, err := btcAmountToSatoshi(tt.amount)
		if tt.valid {
			if err != nil {
				t.Fatalf("btcAmountToSatoshi(%q) returned error: %v", tt.amount, err)
			}
			if got != tt.want {
				t.Fatalf("btcAmountToSatoshi(%q) = %d, want %d", tt.amount, got, tt.want)
			}
			continue
		}
		if err == nil {
			t.Fatalf("btcAmountToSatoshi(%q) unexpectedly succeeded with %d", tt.amount, got)
		}
	}
}
