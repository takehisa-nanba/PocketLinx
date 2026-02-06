package container

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
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

// LoadCache attempts to restore a layer from WSL cache
func (b *WSLBackend) LoadCache(hash string, rootfs string) (bool, error) {
	cacheFile := path.Join(GetWslCacheDir(), hash+".tar.gz")

	// Check if cache file exists
	if err := b.wslClient.RunDistroCommand("test", "-f", cacheFile); err != nil {
		return false, nil // Cache miss
	}

	// Extract cache to rootfs clearing old content?
	// Ideally we layer on top, but since we are taking full snapshots:
	// We need to replace current rootfs with cached one.
	// IMPORTANT: Use safe extraction.
	// Since we are snapshotting the WHOLE rootfs, executing "tar -xf cache.tar.gz -C rootfs"
	// will effectively restore the state.
	// But we should clean rootfs first to avoid ghost files from previous step?
	// -> Actually, in the loop, we are building incrementally.
	// If step N is cached, we need the state of step N.
	// If step N-1 was fresh, rootfs has N-1 state.
	// If we find cache for N, does it contain the delta or the full state?
	// -> For simplicity v0.3.0: FULL SNAPSHOT.
	// So we can just blast the cache content over the directory (or clean and restore).
	// To be safe: clean and restore.

	// Only safe if we are sure the cache is valid.
	b.wslClient.RunDistroCommand("rm", "-rf", rootfs+"/*")
	if err := b.wslClient.RunDistroCommand("tar", "-xf", cacheFile, "-C", rootfs); err != nil {
		return false, err
	}

	return true, nil
}

// SaveCache checkpoints the current rootfs state to WSL cache
func (b *WSLBackend) SaveCache(hash string, rootfs string) error {
	cacheDir := GetWslCacheDir()
	cacheFile := path.Join(cacheDir, hash+".tar.gz")

	// Ensure cache dir exists
	if err := b.wslClient.RunDistroCommand("mkdir", "-p", cacheDir); err != nil {
		return err
	}

	// Create hash file from rootfs
	// Use "." to archive relative to -C rootfs
	return b.wslClient.RunDistroCommand("tar", "-czf", cacheFile, "-C", rootfs, ".")
}

// Prune removes all cached layers
func (b *WSLBackend) Prune() error {
	return b.wslClient.RunDistroCommand("rm", "-rf", GetWslCacheDir()+"/*")
}
