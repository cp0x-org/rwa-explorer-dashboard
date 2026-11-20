package model

import (
	"regexp"
	"strings"
)

// EVMAddress matches a 20-byte hex address.
var EVMAddress = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

// NormalizeAddress lower-cases EVM addresses (case-insensitive); Solana, TON and other formats are
// case-sensitive and returned unchanged.
func NormalizeAddress(address string) string {
	if EVMAddress.MatchString(address) {
		return strings.ToLower(address)
	}
	return address
}
