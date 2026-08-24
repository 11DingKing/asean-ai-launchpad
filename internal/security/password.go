package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    uint32 = 3
	argonMemory  uint32 = 64 * 1024
	argonThreads uint8  = 2
	argonKeyLen  uint32 = 32
)

func HashPassword(password string) (string, error) {
	if len(password) < 12 || len(password) > 256 {
		return "", errors.New("password must contain 12 to 256 characters")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("create password salt: %w", err)
	}
	digest := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", argonMemory, argonTime, argonThreads, base64.RawURLEncoding.EncodeToString(salt), base64.RawURLEncoding.EncodeToString(digest)), nil
}

func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != "argon2id" || parts[1] != "v=19" {
		return false
	}
	parameters := strings.Split(parts[2], ",")
	if len(parameters) != 3 {
		return false
	}
	memory, err := parseParameter(parameters[0], "m=")
	if err != nil || memory != uint64(argonMemory) {
		return false
	}
	timeCost, err := parseParameter(parameters[1], "t=")
	if err != nil || timeCost != uint64(argonTime) {
		return false
	}
	threads, err := parseParameter(parameters[2], "p=")
	if err != nil || threads != uint64(argonThreads) {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(salt) != 16 {
		return false
	}
	want, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil || len(want) != int(argonKeyLen) {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(timeCost), uint32(memory), uint8(threads), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func parseParameter(value, prefix string) (uint64, error) {
	if !strings.HasPrefix(value, prefix) {
		return 0, errors.New("invalid password parameter")
	}
	return strconv.ParseUint(strings.TrimPrefix(value, prefix), 10, 32)
}
