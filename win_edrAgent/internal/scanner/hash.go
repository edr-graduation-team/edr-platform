// Package scanner provides lightweight file hashing for local threat checks.
package scanner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// FileSHA256Limited reads at most maxBytes from path and returns hex-encoded SHA-256 and bytes read.
func FileSHA256Limited(path string, maxBytes int64) (hashHex string, readBytes int64, err error) {
	if maxBytes <= 0 {
		maxBytes = 10 << 20
	}
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return "", 0, err
	}
	if st.IsDir() {
		return "", 0, fmt.Errorf("scanner: path is directory")
	}
	if st.Size() == 0 {
		sum := sha256.Sum256(nil)
		return hex.EncodeToString(sum[:]), 0, nil
	}

	h := sha256.New()
	lr := io.LimitReader(f, maxBytes)
	n, err := io.Copy(h, lr)
	if err != nil {
		return "", n, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
