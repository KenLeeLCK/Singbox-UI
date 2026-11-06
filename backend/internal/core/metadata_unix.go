//go:build linux || darwin

package core

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"strings"
)

func readMetadata(path string) (map[string][]byte, error) {
	n, err := unix.Listxattr(path, nil)
	if errors.Is(err, unix.ENOTSUP) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect extended metadata: %w", err)
	}
	if n > 1<<20 {
		return nil, fmt.Errorf("extended metadata too large")
	}
	buf := make([]byte, n)
	n, err = unix.Listxattr(path, buf)
	if err != nil {
		return nil, err
	}
	result := map[string][]byte{}
	for _, key := range strings.Split(string(buf[:n]), "\x00") {
		if key == "" {
			continue
		}
		size, err := unix.Getxattr(path, key, nil)
		if err != nil {
			return nil, fmt.Errorf("read metadata %s: %w", key, err)
		}
		if size > 1<<20 {
			return nil, fmt.Errorf("metadata %s too large", key)
		}
		value := make([]byte, size)
		size, err = unix.Getxattr(path, key, value)
		if err != nil {
			return nil, err
		}
		result[key] = value[:size]
	}
	return result, nil
}
func setMetadata(path string, attrs map[string][]byte) error {
	if attrs == nil {
		return nil
	}
	inherited, err := readMetadata(path)
	if err != nil {
		return err
	}
	for key := range inherited {
		if _, ok := attrs[key]; !ok {
			if err := unix.Removexattr(path, key); err != nil {
				return fmt.Errorf("cannot clear inherited metadata %s: %w", key, err)
			}
		}
	}

	for key, value := range attrs {
		if err := unix.Setxattr(path, key, value, 0); err != nil {
			return fmt.Errorf("cannot preserve metadata %s; refusing replacement: %w", key, err)
		}
	}
	return nil
}
