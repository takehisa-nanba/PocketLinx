package wslbridge

import (
	"encoding/hex"
	"fmt"
	"hash"
	"io"
)

type receiver struct {
	file   io.Writer
	hash   hash.Hash
	header []byte
	size   int64
}

func (r *receiver) Write(p []byte) (int, error) {
	original := len(p)
	if len(r.header) < 65 {
		n := 65 - len(r.header)
		if n > len(p) {
			n = len(p)
		}
		r.header = append(r.header, p[:n]...)
		p = p[n:]
		if len(r.header) == 65 {
			if r.header[64] != '\n' {
				return 0, fmt.Errorf("invalid transfer header")
			}
			if _, err := hex.DecodeString(string(r.header[:64])); err != nil {
				return 0, err
			}
		}
	}
	if r.size+int64(len(p)) > MaxTransferBytes {
		return 0, fmt.Errorf("transfer size limit exceeded")
	}
	n, err := r.file.Write(p)
	r.hash.Write(p[:n])
	r.size += int64(n)
	if err != nil {
		return original - len(p) + n, err
	}
	if n != len(p) {
		return original - len(p) + n, io.ErrShortWrite
	}
	return original, nil
}
func (r *receiver) finish() error {
	if len(r.header) != 65 || r.size == 0 || string(r.header[:64]) != hex.EncodeToString(r.hash.Sum(nil)) {
		return fmt.Errorf("transfer SHA-256 mismatch or truncated stream")
	}
	return nil
}
