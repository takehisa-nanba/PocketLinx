package container

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// GetWslCacheDir returns the directory where intermediate layers are stored inside WSL
func GetWslCacheDir() string {
	return "/var/lib/pocketlinx/cache"
}

// CalculateInstructionHash computes a deterministic hash for a build step
func CalculateInstructionHash(parentHash string, instr Instruction, ctxDir string) (string, error) {
	hasher := sha256.New()

	// Mix in parent hash (chaining)
	hasher.Write([]byte(parentHash))

	// Mix in instruction type and raw content
	hasher.Write([]byte(instr.Type))
	hasher.Write([]byte(instr.Raw))

	// For COPY, we must hash the actual file contents
	if instr.Type == "COPY" && len(instr.Args) >= 2 {
		// instr.Args[0] is source (relative to ctxDir)
		srcPath := filepath.Join(ctxDir, instr.Args[0])
		fileHash, err := hashPath(srcPath)
		if err != nil {
			return "", fmt.Errorf("failed to hash copy source %s: %w", srcPath, err)
		}
		hasher.Write([]byte(fileHash))
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// hashPath computes SHA256 of a file or directory recursively
func hashPath(pathStr string) (string, error) {
	hasher := sha256.New()
	err := filepath.WalkDir(pathStr, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Hash the relative path and mode
		relPath, _ := filepath.Rel(pathStr, p)
		fmt.Fprintf(hasher, "%s|%v|", relPath, d.IsDir())

		if !d.IsDir() {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(hasher, f); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
