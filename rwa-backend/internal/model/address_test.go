package model

import "testing"

func TestNormalizeAddress(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0xB20000000000000000000078EE7ce2fe4908108C", "0xb20000000000000000000078ee7ce2fe4908108c"},
		{"0xb20000000000000000000078ee7ce2fe4908108c", "0xb20000000000000000000078ee7ce2fe4908108c"},
		{"Xsc9qvGR1efVDFGLrVsmkzv3qi45LTBjeUKSPmx9qEh", "Xsc9qvGR1efVDFGLrVsmkzv3qi45LTBjeUKSPmx9qEh"}, // Solana: case-sensitive
		{"0xABC", "0xABC"}, // too short
		{"0xZZ0000000000000000000078EE7ce2fe4908108C", "0xZZ0000000000000000000078EE7ce2fe4908108C"}, // not hex
		{"0XB20000000000000000000078EE7ce2fe4908108C", "0XB20000000000000000000078EE7ce2fe4908108C"}, // uppercase prefix is not EVM here
		{" 0xB20000000000000000000078EE7ce2fe4908108C", " 0xB20000000000000000000078EE7ce2fe4908108C"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := NormalizeAddress(tc.in); got != tc.want {
			t.Errorf("NormalizeAddress(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
