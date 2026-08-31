package passwords

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	algorithm       = "pbkdf2-sha256"
	iterations      = 600000
	saltBytes       = 32
	derivedKeyBytes = 32
)

func Hash(password string) (string, error) {
	if len(password) < 12 {
		return "", errors.New("password must be at least 12 characters")
	}

	salt := make([]byte, saltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	derived := derive([]byte(password), salt, iterations, derivedKeyBytes)
	encoding := base64.RawURLEncoding
	return fmt.Sprintf("%s$%d$%s$%s",
		algorithm,
		iterations,
		encoding.EncodeToString(salt),
		encoding.EncodeToString(derived),
	), nil
}

func Verify(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != algorithm {
		return false
	}
	count, err := strconv.Atoi(parts[1])
	if err != nil || count < 100000 || count > 5000000 {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(salt) < 16 {
		return false
	}
	expected, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(expected) != derivedKeyBytes {
		return false
	}
	actual := derive([]byte(password), salt, count, len(expected))
	return hmac.Equal(actual, expected)
}

func derive(password, salt []byte, count, keyLen int) []byte {
	result := make([]byte, 0, keyLen)
	block := make([]byte, len(salt)+4)
	copy(block, salt)

	for blockIndex := uint32(1); len(result) < keyLen; blockIndex++ {
		block[len(salt)] = byte(blockIndex >> 24)
		block[len(salt)+1] = byte(blockIndex >> 16)
		block[len(salt)+2] = byte(blockIndex >> 8)
		block[len(salt)+3] = byte(blockIndex)

		mac := hmac.New(sha256.New, password)
		_, _ = mac.Write(block)
		previous := mac.Sum(nil)
		accumulator := append([]byte(nil), previous...)

		for i := 1; i < count; i++ {
			mac = hmac.New(sha256.New, password)
			_, _ = mac.Write(previous)
			previous = mac.Sum(nil)
			for j := range accumulator {
				accumulator[j] ^= previous[j]
			}
		}
		result = append(result, accumulator...)
	}
	return result[:keyLen]
}
