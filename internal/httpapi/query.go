package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/11DingKing/asean-ai-launchpad/internal/domain"
)

func pageFrom(r *http.Request) (domain.Page, error) {
	page := domain.Page{}
	var err error
	if raw := r.URL.Query().Get("limit"); raw != "" {
		page.Limit, err = strconv.Atoi(raw)
		if err != nil {
			return domain.Page{}, domain.ErrInvalid
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		page.Offset, err = strconv.Atoi(raw)
		if err != nil {
			return domain.Page{}, domain.ErrInvalid
		}
	}
	return page.Normalize(), nil
}

func timeQuery(r *http.Request, name string) (time.Time, error) {
	raw := r.URL.Query().Get(name)
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, domain.ErrInvalid
	}
	return parsed.UTC(), nil
}
