//go:build !linux && !darwin

package core

import "errors"

func readMetadata(string) (map[string][]byte, error) {
	return nil, errors.New("configuration replacement unsupported on this OS: cannot preserve metadata")
}
func setMetadata(string, map[string][]byte) error {
	return errors.New("metadata preservation unavailable")
}
