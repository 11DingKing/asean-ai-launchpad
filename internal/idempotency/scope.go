package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func Scope(actorID, method, path, key string) (string, error) {
	actorID = strings.TrimSpace(actorID)
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	key = strings.TrimSpace(key)
	if actorID == "" || method == "" || path == "" || len(key) < 8 || len(key) > 200 {
		return "", fmt.Errorf("invalid idempotency scope")
	}
	digest := sha256.Sum256([]byte(actorID + "\x00" + method + "\x00" + path + "\x00" + key))
	return hex.EncodeToString(digest[:]), nil
}

func RequestHash(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
