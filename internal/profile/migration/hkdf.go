package migration

import (
	"crypto/hmac"
	"crypto/sha256"
)

func hkdfSHA256(secret, salt, info []byte, length int) []byte {
	extractor := hmac.New(sha256.New, salt)
	extractor.Write(secret)
	prk := extractor.Sum(nil)
	defer zero(prk)
	result := make([]byte, 0, length)
	previous := []byte{}
	for counter := byte(1); len(result) < length; counter++ {
		expander := hmac.New(sha256.New, prk)
		expander.Write(previous)
		expander.Write(info)
		expander.Write([]byte{counter})
		previous = expander.Sum(nil)
		result = append(result, previous...)
	}
	zero(previous)
	return result[:length]
}
