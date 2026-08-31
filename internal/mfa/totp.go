package mfa

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	defaultSecretBytes = 20
	defaultPeriod      = 30
	defaultDigits      = 6
)

func NewSecret() (string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "AI Token Gateway",
		AccountName: "enrollment",
		Period:      defaultPeriod,
		SecretSize:  defaultSecretBytes,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", err
	}
	return key.Secret(), nil
}

func Code(secret string, at time.Time, digits int) (string, error) {
	if digits < 6 || digits > 8 {
		return "", errors.New("totp digits must be between 6 and 8")
	}
	return totp.GenerateCodeCustom(secret, at, totp.ValidateOpts{
		Period:    defaultPeriod,
		Digits:    otp.Digits(digits),
		Algorithm: otp.AlgorithmSHA1,
	})
}

func Verify(secret, candidate string, at time.Time, window int) bool {
	if window < 0 || len(candidate) < 6 || len(candidate) > 8 {
		return false
	}
	for _, char := range candidate {
		if char < '0' || char > '9' {
			return false
		}
	}

	verified, err := totp.ValidateCustom(candidate, secret, at, totp.ValidateOpts{
		Period:    defaultPeriod,
		Skew:      uint(window),
		Digits:    otp.Digits(len(candidate)),
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && verified
}

func decodeSecret(secret string) ([]byte, error) {
	normalized := strings.ToUpper(strings.TrimSpace(secret))
	if normalized == "" {
		return nil, errors.New("totp secret is empty")
	}
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.TrimRight(normalized, "="))
	if err != nil {
		return nil, errors.New("invalid totp secret")
	}
	return decoded, nil
}

type SecretBox struct {
	aead cipher.AEAD
}

func NewSecretBox(key []byte) (*SecretBox, error) {
	if len(key) != 32 {
		return nil, errors.New("secret box key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{aead: aead}, nil
}

func (b *SecretBox) Seal(plaintext []byte) (string, error) {
	if b == nil || b.aead == nil {
		return "", errors.New("secret box is not configured")
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := b.aead.Seal(nil, nonce, plaintext, nil)
	payload := append(nonce, sealed...)
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func (b *SecretBox) Open(encoded string) ([]byte, error) {
	if b == nil || b.aead == nil {
		return nil, errors.New("secret box is not configured")
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(payload) < b.aead.NonceSize() {
		return nil, errors.New("invalid sealed secret")
	}
	nonce := payload[:b.aead.NonceSize()]
	ciphertext := payload[b.aead.NonceSize():]
	plaintext, err := b.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("invalid sealed secret")
	}
	return plaintext, nil
}
