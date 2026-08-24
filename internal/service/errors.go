package service

import (
	"errors"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func isNotFound(err error) bool {
	return errors.Is(err, domain.ErrNotFound)
}
