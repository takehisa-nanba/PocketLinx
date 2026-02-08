package container

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// hashPath computes SHA256 of a file or directory recursively, respecting .plxignore
func hashPath(pathStr string) (string, error) {
	hasher := sha256.New()

	// Load ignore patterns if .plxignore exists in the context root
	ignorePatterns := make(map[string]bool)
	ignoreFile := filepath.Join(pathStr, ".plxignore")
	if data, err := os.ReadFile(ignoreFile); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				ignorePatterns[line] = true
			}
		}
	}

	// Always ignore these
	ignorePatterns[".git"] = true

	count := 0
	err := filepath.WalkDir(pathStr, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(pathStr, p)
		if relPath == "." {
			return nil
		}

		// Check if this path should be ignored
		if ignorePatterns[relPath] || ignorePatterns[filepath.Base(p)] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		count++
		if os.Getenv("PLX_VERBOSE") != "" && count%100 == 0 {
			fmt.Printf("\r[DEBUG] Hashing context: %d files scanned...", count)
		}

		// Hash the relative path and mode
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
	if os.Getenv("PLX_VERBOSE") != "" && count > 0 {
		fmt.Println()
	}
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
