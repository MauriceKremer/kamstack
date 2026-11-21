package main

import (
	"crypto/rand"
	"math/big"
)

// passwordAlphabet avoids characters that are easy to confuse when a person
// copies a generated password from a terminal.
const passwordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomPassword(length int) (string, error) {
	out := make([]byte, length)
	limit := big.NewInt(int64(len(passwordAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		out[i] = passwordAlphabet[n.Int64()]
	}
	return string(out), nil
}
