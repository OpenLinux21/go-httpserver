package utils

import (
	"math/rand"
	"time"
)

const randomStringCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func GenerateRandomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	randomBytes := make([]byte, length)
	for i := range randomBytes {
		randomBytes[i] = randomStringCharset[rand.Intn(len(randomStringCharset))]
	}
	return string(randomBytes)
}
