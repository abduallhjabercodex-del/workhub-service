package domain

import (
	"errors"
	"fmt"
)

var (
	ErrQuotaExceeded = errors.New("quota_exceeded")
)

type QuotaError struct {
	Resource string
	Current  int
	Max      int
}

func (e *QuotaError) Error() string {
	return fmt.Sprintf("quota exceeded for %s: %d/%d allowed", e.Resource, e.Current, e.Max)
}

func (e *QuotaError) Is(target error) bool {
	return target == ErrQuotaExceeded
}
