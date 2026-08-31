package tokens

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	livePrefix  = "sk_live_"
	tokenBytes  = 32
	prefixBytes = 6
)

var ErrInvalidToken = errors.New("invalid api token")

type Hasher struct {
	pepper []byte
}

func NewHasher(pepper string) (*Hasher, error) {
	if len(pepper) < 32 {
		return nil, errors.New("token pepper must be at least 32 bytes")
	}
	return &Hasher{pepper: []byte(pepper)}, nil
}

func (h *Hasher) Generate() (plain string, prefix string, digest string, err error) {
	if h == nil || len(h.pepper) < 32 {
		return "", "", "", errors.New("token hasher is not configured")
	}

	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", "", err
	}

	encoded := base64.RawURLEncoding.EncodeToString(raw)
	plain = livePrefix + encoded
	prefix = plain[:len(livePrefix)+prefixBytes]
	digest = h.Digest(plain)
	return plain, prefix, digest, nil
}

func (h *Hasher) Digest(plain string) string {
	mac := hmac.New(sha256.New, h.pepper)
	_, _ = mac.Write([]byte(plain))
	return hex.EncodeToString(mac.Sum(nil))
}

func (h *Hasher) Verify(plain, expectedDigest string) bool {
	if h == nil || len(h.pepper) < 32 || !validFormat(plain) || expectedDigest == "" {
		return false
	}
	actual := h.Digest(plain)
	return hmac.Equal([]byte(actual), []byte(strings.ToLower(expectedDigest)))
}

func validFormat(plain string) bool {
	if !strings.HasPrefix(plain, livePrefix) {
		return false
	}
	raw := strings.TrimPrefix(plain, livePrefix)
	if len(raw) != base64.RawURLEncoding.EncodedLen(tokenBytes) {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(raw)
	return err == nil
}
