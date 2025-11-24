package utils

import (
	"crypto/rand"
	"math/big"
	mrand "math/rand"
	"sync"
	"time"
)

var (
	fallbackRand   = mrand.New(mrand.NewSource(time.Now().UnixNano()))
	fallbackRandMu sync.Mutex
)

const randomStringCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func GenerateRandomString(length int) string {
	randomBytes := make([]byte, length)
	charsetLength := int64(len(randomStringCharset))

	for i := range randomBytes {
		// Prefer cryptographically secure randomness for log identifiers.
		n, err := rand.Int(rand.Reader, big.NewInt(charsetLength))
		if err == nil {
			randomBytes[i] = randomStringCharset[n.Int64()]
			continue
		}

		// Fallback to math/rand with a pre-seeded generator only if
		// crypto/rand fails, guarding access to ensure concurrency safety.
		fallbackRandMu.Lock()
		randomBytes[i] = randomStringCharset[fallbackRand.Intn(int(charsetLength))]
		fallbackRandMu.Unlock()
	}

	return string(randomBytes)
}
