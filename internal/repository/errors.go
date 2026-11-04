package repository

import "errors"

var ErrNotFound = errors.New("location not found")

var ErrConflict = errors.New("location id already exists with different data")
