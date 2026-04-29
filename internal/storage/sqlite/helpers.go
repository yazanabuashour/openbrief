package sqlite

import (
	"crypto/rand"
	"encoding/hex"
)

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func newRunID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[:]), nil
}
