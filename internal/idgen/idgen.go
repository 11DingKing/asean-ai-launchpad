package idgen

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
)

type Generator interface {
	New(prefix string) (string, error)
}

type Random struct{}

func (Random) New(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	if prefix == "" {
		return hex.EncodeToString(raw[:]), nil
	}
	return prefix + "_" + hex.EncodeToString(raw[:]), nil
}

type Sequence struct {
	mu   sync.Mutex
	Next int
}

func (s *Sequence) New(prefix string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Next++
	return fmt.Sprintf("%s_%06d", prefix, s.Next), nil
}
