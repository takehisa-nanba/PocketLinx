//go:build windows

package container

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"PocketLinx/pkg/shim"
	"PocketLinx/pkg/wsl"
)

// WSLImageService implements ImageService using WSL2
type WSLImageService struct {
	wslClient *wsl.Client
}

func NewWSLImageService(client *wsl.Client) *WSLImageService {
	return &WSLImageService{wslClient: client}
}

func (s *WSLImageService) Images() ([]string, error) {
	// List files in WSL images directory
	cmd := fmt.Sprintf("ls %s/*.tar.gz", GetWslImagesDir())
	out, err := s.wslClient.RunDistroCommandOutput("sh", "-c", cmd)
	if err != nil {
		// If explicit ls fails, it might mean empty dir or no files
		return []string{}, nil
	}

	var images []string
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		// /var/lib/pocketlinx/images/alpine.tar.gz -> alpine
		base := path.Base(l)
		if strings.HasSuffix(base, ".tar.gz") {
			images = append(images, strings.TrimSuffix(base, ".tar.gz"))
		}
	}
	return images, nil
}

func (s *WSLImageService) Pull(image string) error {
	url, ok := SupportedImages[image]
	if !ok {
		return fmt.Errorf("image '%s' is not supported", image)
	}

	wslImagesDir := GetWslImagesDir()

	// Special handling for System Distro bootstrap (Alpine)
	if image == "alpine" {
		targetFile := filepath.Join(GetImagesDir(), image+".tar.gz")
		if _, err := os.Stat(targetFile); err != nil {
			fmt.Printf("Downloading bootstrap image '%s'...\n", image)
			cmd := exec.Command("powershell.exe", "-Command", fmt.Sprintf("Invoke-WebRequest -Uri %s -OutFile %s", url, targetFile))
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("error downloading bootstrap image: %w", err)
			}
		}

		installDir := GetDistroDir()
		absInstallDir, _ := filepath.Abs(installDir)
		absRootfsFile, _ := filepath.Abs(targetFile)

		fmt.Printf("Importing system distro '%s'...\n", DistroName)
		s.wslClient.Run("--unregister", DistroName)
		os.RemoveAll(absInstallDir)
		os.MkdirAll(absInstallDir, 0755)

		if err := s.wslClient.Run("--import", DistroName, absInstallDir, absRootfsFile, "--version", "2"); err != nil {
			return fmt.Errorf("error importing system distro: %w", err)
		}

		fmt.Println("Installing container-shim...")
		if err := s.wslClient.RunDistroCommandWithInput(shim.Content, "sh", "-c", "cat > /usr/local/bin/container-shim && chmod +x /usr/local/bin/container-shim"); err != nil {
			return err
		}

		// Distro exists now. Ensure directory exists inside WSL
		if err := s.wslClient.RunDistroCommand("mkdir", "-p", wslImagesDir); err != nil {
			return fmt.Errorf("failed to create images dir in WSL: %w", err)
		}

		// Cache this image into WSL storage for Run/Build to use
		fmt.Println("Caching bootstrap image to WSL storage...")
		wslWinPath, _ := wsl.WindowsToWslPath(targetFile)
		targetWslFile := path.Join(wslImagesDir, image+".tar.gz")
		s.wslClient.RunDistroCommand("cp", wslWinPath, targetWslFile)
		return nil
	}

	// Normal flow for other images (Distro assumed to exist)
	// Ensure directory exists
	if err := s.wslClient.RunDistroCommand("mkdir", "-p", wslImagesDir); err != nil {
		return fmt.Errorf("failed to create images dir in WSL (is PocketLinx setup?): %w", err)
	}
	targetWslFile := path.Join(wslImagesDir, image+".tar.gz")

	// Check if exists in WSL
	if err := s.wslClient.RunDistroCommand("test", "-f", targetWslFile); err == nil {
		fmt.Printf("Image '%s' already exists.\n", image)
		return nil
	}

	// Native Download for other images
	fmt.Printf("Pulling image '%s' inside WSL...\n", image)
	downloadCmd := fmt.Sprintf("wget -O %s %s || curl -L -o %s %s", targetWslFile, url, targetWslFile, url)
	if err := s.wslClient.RunDistroCommand("sh", "-c", downloadCmd); err != nil {
		return fmt.Errorf("error downloading image in WSL: %w", err)
	}

	return nil
}

func (s *WSLImageService) Build(ctxDir string, tag string) (string, error) {
	dockerfilePath := filepath.Join(ctxDir, "Dockerfile")
	df, err := ParseDockerfile(dockerfilePath)
	if err != nil {
		return "", fmt.Errorf("failed to parse Dockerfile: %w", err)
	}

	// 1. Calculate Hash Chain to determine where to resume
	// parentHash starts with the Base Image + "FROM"
	parentHash := fmt.Sprintf("%x", sha256.Sum256([]byte("FROM "+df.Base)))

	// Hashes for each instruction index
	stepHashes := make([]string, len(df.Instructions))

	for i, instr := range df.Instructions {
		h, err := CalculateInstructionHash(parentHash, instr, ctxDir)
		if err != nil {
			return "", fmt.Errorf("failed to calculate hash for step %d: %w", i, err)
		}
		stepHashes[i] = h
		parentHash = h
	}

	// 2. Find the last cache hit (Fast Forward)
	lastHitIndex := -1
	for i := len(stepHashes) - 1; i >= 0; i-- {
		cacheFile := path.Join(GetWslCacheDir(), stepHashes[i]+".tar.gz")
		if err := s.wslClient.RunDistroCommand("test", "-f", cacheFile); err == nil {
			lastHitIndex = i
			break
		}
	}

	// 3. Prepare Build Directory
	imageName := tag
	if imageName == "" {
		imageName = strings.ToLower(filepath.Base(ctxDir))
	}
	fmt.Printf("Building image '%s' from %s...\n", imageName, df.Base)

	buildId := fmt.Sprintf("build-%d", os.Getpid())
	buildDir := fmt.Sprintf("/var/lib/pocketlinx/builds/%s", buildId)
	rootfsDir := path.Join(buildDir, "rootfs")
	s.wslClient.RunDistroCommand("mkdir", "-p", rootfsDir)
	defer s.wslClient.RunDistroCommand("rm", "-rf", buildDir)

	// 4. Restore state OR Initialize Base
	if lastHitIndex >= 0 {
		hitHash := stepHashes[lastHitIndex]
		fmt.Printf("CACHED: Resuming from step %d (Hash: %s)\n", lastHitIndex+1, hitHash[:12])
		if _, err := s.LoadCache(hitHash, rootfsDir); err != nil {
			return "", fmt.Errorf("failed to load cache %s: %w", hitHash, err)
		}
	} else {
		// Initialize from Base Image
		baseTarWsl := path.Join(GetWslImagesDir(), df.Base+".tar.gz")
		if err := s.wslClient.RunDistroCommand("test", "-f", baseTarWsl); err != nil {
			fmt.Printf("Base image not found, pulling %s...\n", df.Base)
			if err := s.Pull(df.Base); err != nil {
				return "", fmt.Errorf("failed to pull base image %s: %w", df.Base, err)
			}
		}
		if err := s.wslClient.RunDistroCommand("tar", "-xf", baseTarWsl, "-C", rootfsDir); err != nil {
			return "", fmt.Errorf("failed to extract base image: %w", err)
		}
	}

	// 5. Execute Remaining Steps
	currentWorkdir := "/"
	envPrefix := "" // Need to reconstruct ENV from scratch or cache?
	// PROBLEM: Reconstruct ENV if we skipped steps!
	// We must iterate ALL instructions to build up ENV, even if we skip execution.

	for i, instr := range df.Instructions {
		// Always process ENV state
		if instr.Type == "ENV" {
			for j := 0; j < len(instr.Args); j += 2 {
				k := instr.Args[j]
				v := ""
				if j+1 < len(instr.Args) {
					v = instr.Args[j+1]
				}
				envPrefix += fmt.Sprintf("export %s=%q; ", k, v)
			}
		}

		// Skip execution if covered by cache
		if i <= lastHitIndex {
			if instr.Type == "WORKDIR" && len(instr.Args) > 0 {
				currentWorkdir = instr.Args[0]
			}
			continue
		}

		// Execute Step
		fmt.Printf("[%d/%d] %s %s\n", i+1, len(df.Instructions), instr.Type, instr.Raw)

		switch instr.Type {
		// ENV is already handled above for prefix string
		case "ENV":
			fmt.Println("Applying ENV...")
			// No FS change usually, but we treat it as a step that creates a layer

		case "RUN":
			if err := s.executeBuildRun(envPrefix, instr.Raw, rootfsDir, currentWorkdir); err != nil {
				return "", fmt.Errorf("RUN failed: %w", err)
			}

		case "COPY":
			if err := s.executeBuildCopy(ctxDir, instr.Args[0], instr.Args[1], rootfsDir, currentWorkdir); err != nil {
				return "", fmt.Errorf("COPY failed: %w", err)
			}

		case "WORKDIR":
			if len(instr.Args) > 0 {
				currentWorkdir = instr.Args[0]
				workdirPath := path.Join(rootfsDir, strings.TrimPrefix(currentWorkdir, "/"))
				s.wslClient.RunDistroCommand("mkdir", "-p", workdirPath)
			}
		}

		// Save Cache after execution
		stepHash := stepHashes[i]
		if err := s.SaveCache(stepHash, rootfsDir); err != nil {
			fmt.Printf("Warning: Failed to save cache for step %d: %v\n", i, err)
		}
	}

	// 6. Final Save
	outputTarWsl := path.Join(GetWslImagesDir(), imageName+".tar.gz")

	fmt.Printf("Saving image to %s (WSL)...\n", outputTarWsl)
	if err := s.wslClient.RunDistroCommand("tar", "-czf", outputTarWsl, "-C", rootfsDir, "."); err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	fmt.Printf("Successfully built image '%s'\n", imageName)
	return imageName, nil
}

func (s *WSLImageService) executeBuildRun(envPrefix string, runCmd string, rootfsDir string, currentWorkdir string) error {
	fmt.Printf("STEP: RUN %s\n", runCmd)

	// 3. Create a temporary script for the command to avoid quoting issues
	scriptName := fmt.Sprintf("build_step_%d.sh", time.Now().UnixNano())

	// Use ToSlash to ensure Linux-style paths for WSL commands
	tmpDir := filepath.ToSlash(filepath.Join(rootfsDir, "tmp"))
	scriptDstPath := filepath.ToSlash(filepath.Join(tmpDir, scriptName))

	// Ensure tmp exists
	if err := s.wslClient.RunDistroCommand("mkdir", "-p", tmpDir); err != nil {
		return fmt.Errorf("failed to creating tmp dir: %w", err)
	}

	// Write the script
	scriptContent := fmt.Sprintf("#!/bin/sh\nset -e\n%s\n%s", envPrefix, runCmd)

	// Write using cat
	cmd := exec.Command("wsl", "-d", s.wslClient.DistroName, "sh", "-c", fmt.Sprintf("cat > %s", scriptDstPath))
	cmd.Stdin = strings.NewReader(scriptContent)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write build script: %w", err)
	}

	// Make executable
	if err := s.wslClient.RunDistroCommand("chmod", "+x", scriptDstPath); err != nil {
		return fmt.Errorf("failed to chmod script: %w", err)
	}

	// 隔離環境のセットアップと実行
	fullCmd := fmt.Sprintf(
		"mkdir -p %s/proc %s/sys %s/dev %s%s && "+
			"mount -t proc proc %s/proc && "+
			"mount -t sysfs sys %s/sys && "+
			"rm -f %s/dev/null %s/dev/zero %s/dev/random %s/dev/urandom && "+
			"mknod -m 666 %s/dev/null c 1 3 && "+
			"mknod -m 666 %s/dev/zero c 1 5 && "+
			"mknod -m 666 %s/dev/random c 1 8 && "+
			"mknod -m 666 %s/dev/urandom c 1 9 && "+
			"mkdir -p %s/etc && "+
			"cat /etc/resolv.conf > %s/etc/resolv.conf && "+
			"chroot %s /tmp/%s; "+
			"RET=$?; umount %s/proc %s/sys; exit $RET",
		rootfsDir, rootfsDir, rootfsDir, rootfsDir, currentWorkdir,
		rootfsDir, rootfsDir,
		rootfsDir, rootfsDir, rootfsDir, rootfsDir, // for rm -f
		rootfsDir, rootfsDir, rootfsDir, rootfsDir, // for mknod
		rootfsDir, // for mkdir -p %s/etc
		rootfsDir, // for cat /etc/resolv.conf > ...
		rootfsDir, scriptName,
		rootfsDir, rootfsDir,
	)

	return s.wslClient.RunDistroCommand("unshare", "--mount", "sh", "-c", fullCmd)
}

func (s *WSLImageService) executeBuildCopy(ctxDir, srcArg, destArg, rootfsDir, currentWorkdir string) error {
	src := filepath.Join(ctxDir, srcArg)
	dest := path.Join(rootfsDir, strings.TrimPrefix(path.Join(currentWorkdir, destArg), "/"))
	fmt.Printf("STEP: COPY %s to %s\n", srcArg, destArg)

	srcWsl, err := wsl.WindowsToWslPath(src)
	if err != nil {
		return fmt.Errorf("failed to convert src path: %w", err)
	}

	// ホストからrootfs内へコピー
	// mkdir -p で親ディレクトリを作成し、cp -r で再帰的にコピー
	cmd := fmt.Sprintf("mkdir -p $(dirname %s) && cp -r %s %s", dest, srcWsl, dest)
	return s.wslClient.RunDistroCommand("sh", "-c", cmd)
}

// Prune removes all cached layers
func (s *WSLImageService) Prune() error {
	return s.wslClient.RunDistroCommand("rm", "-rf", GetWslCacheDir()+"/*")
}

// LoadCache attempts to restore a layer from WSL cache
func (s *WSLImageService) LoadCache(hash string, rootfs string) (bool, error) {
	cacheFile := path.Join(GetWslCacheDir(), hash+".tar.gz")

	// Check if cache file exists
	if err := s.wslClient.RunDistroCommand("test", "-f", cacheFile); err != nil {
		return false, nil // Cache miss
	}

	// See pkg/container/cache.go for comments
	s.wslClient.RunDistroCommand("rm", "-rf", rootfs+"/*")
	if err := s.wslClient.RunDistroCommand("tar", "-xf", cacheFile, "-C", rootfs); err != nil {
		return false, err
	}

	return true, nil
}

// SaveCache checkpoints the current rootfs state to WSL cache
func (s *WSLImageService) SaveCache(hash string, rootfs string) error {
	cacheDir := GetWslCacheDir()
	cacheFile := path.Join(cacheDir, hash+".tar.gz")

	// Ensure cache dir exists
	if err := s.wslClient.RunDistroCommand("mkdir", "-p", cacheDir); err != nil {
		return err
	}

	// Create hash file from rootfs
	return s.wslClient.RunDistroCommand("tar", "-czf", cacheFile, "-C", rootfs, ".")
}
