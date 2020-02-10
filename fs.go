package main

import (
	"errors"
	"fmt"
	"os"
)

func isDir(path string) (bool, error) {
	switch info, err := os.Stat(path); {
	case err == nil:
		return info.IsDir(), nil
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("stat: %w", err)
	}
}
