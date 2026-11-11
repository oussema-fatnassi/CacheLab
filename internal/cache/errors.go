package cache

import "errors"

var (
	ErrInvalidKey    = errors.New("invalid key")
	ErrValueTooLarge = errors.New("value too large")
	ErrKeyExists     = errors.New("key already exists")
	ErrNotFound      = errors.New("key not found")
)
