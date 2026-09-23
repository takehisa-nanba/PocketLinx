package environment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
)

// LoadDefinition strictly reads the portable definition without executing it.
// v2 requires explicit numeric uid/gid; missing/null must not default to root.
func LoadDefinition(p string) (Definition, error) {
	var d Definition
	f, err := openRegular(p)
	if err != nil {
		return d, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return d, err
	}
	if len(b) > 1<<20 {
		return d, fmt.Errorf("environment.json exceeds 1 MiB")
	}
	if err := decodeJSON(bytes.NewReader(b), &d); err != nil {
		return d, fmt.Errorf("environment.json: %w", err)
	}
	if d.Version == 2 {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(b, &fields); err != nil {
			return d, err
		}
		for _, key := range []string{"uid", "gid"} {
			if len(fields[key]) == 0 || bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
				return d, fmt.Errorf("version 2 requires explicit %s", key)
			}
		}
	}
	return d, d.validate()
}

// InspectDirectory checks a prepared directory against the existing v1 file
// contract and returns its definition. Callers must exclude concurrent writers.
// This deliberately shares the archive inventory validator, not runtime logic.
func InspectDirectory(directory string) (Definition, error) {
	var d Definition
	if err := supported(); err != nil {
		return d, err
	}
	root, err := canonicalDirectory(directory)
	if err != nil {
		return d, err
	}
	if err := rejectMounts(root); err != nil {
		return d, err
	}
	limits, _ := (Limits{}).normalized()
	if _, _, err := inventory(root, limits); err != nil {
		return d, err
	}
	return LoadDefinition(filepath.Join(root, "environment.json"))
}
