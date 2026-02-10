//go:build windows

package container

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
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
	wslClient   *wsl.Client
	currentUser string
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

		// Check if distro already exists (v1.0.7)
		// wsl.exe -l -q returns error if not found
		exists := false
		checkCmd := exec.Command("wsl.exe", "-l", "-q", "-d", s.wslClient.DistroName)
		if err := checkCmd.Run(); err == nil {
			exists = true
			fmt.Printf("System distro '%s' already exists. Keeping existing data.\n", s.wslClient.DistroName)
		}

		if !exists {
			fmt.Printf("Importing system distro '%s'...\n", s.wslClient.DistroName)
			s.wslClient.Run("--unregister", s.wslClient.DistroName)
			os.RemoveAll(absInstallDir)
			os.MkdirAll(absInstallDir, 0755)

			if err := s.wslClient.Run("--import", s.wslClient.DistroName, absInstallDir, absRootfsFile, "--version", "2"); err != nil {
				return fmt.Errorf("error importing system distro: %w", err)
			}
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

func (s *WSLImageService) Build(ctxDir string, dockerfile string, tag string) (string, error) {
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	dockerfilePath := filepath.Join(ctxDir, dockerfile)
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
	allCached := false
	if lastHitIndex >= 0 {
		hitHash := stepHashes[lastHitIndex]
		if lastHitIndex == len(df.Instructions)-1 {
			allCached = true
			fmt.Printf("CACHED: Entire Dockerfile hit cache. Enabling Build Shortcut (instant save).\n")
		} else {
			fmt.Printf("CACHED: Resuming from step %d (Hash: %s)\n", lastHitIndex+1, hitHash[:12])
			if _, err := s.LoadCache(hitHash, rootfsDir); err != nil {
				return "", fmt.Errorf("failed to load cache %s: %w", hitHash, err)
			}
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
		if err := s.wslClient.RunDistroCommand("tar", "-xzf", baseTarWsl, "-C", rootfsDir); err != nil {
			return "", fmt.Errorf("failed to extract base image: %w", err)
		}
	}

	// 5. Execute Remaining Steps
	currentWorkdir := "/"
	s.currentUser = "root" // Reset to root for new build
	envMap := make(map[string]string)
	envPrefix := ""

	for i, instr := range df.Instructions {
		if instr.Type == "ENV" {
			for j := 0; j < len(instr.Args); j += 2 {
				k := instr.Args[j]
				v := ""
				if j+1 < len(instr.Args) {
					v = instr.Args[j+1]
				}
				envMap[k] = v
				envPrefix += fmt.Sprintf("export %s=%q; ", k, v)
			}
		}

		// Update state even if we skip execution because of cache
		if instr.Type == "WORKDIR" && len(instr.Args) > 0 {
			currentWorkdir = instr.Args[0]
		}
		if instr.Type == "USER" && len(instr.Args) > 0 {
			s.currentUser = instr.Args[0]
		}

		// Skip execution if covered by cache
		if i <= lastHitIndex {
			continue
		}

		// Execute Step
		fmt.Printf("[%d/%d] %s %s\n", i+1, len(df.Instructions), instr.Type, instr.Raw)

		isSkippable := false
		switch strings.ToUpper(instr.Type) {
		case "ENV", "USER", "WORKDIR", "LABEL", "COPY", "ADD":
			isSkippable = true
		}

		switch instr.Type {
		case "RUN":
			if err := s.executeBuildRun(envPrefix, instr.Raw, rootfsDir, currentWorkdir); err != nil {
				return "", fmt.Errorf("RUN failed: %w", err)
			}

		case "COPY":
			if err := s.executeBuildCopy(ctxDir, instr.Args[0], instr.Args[1], rootfsDir, currentWorkdir); err != nil {
				return "", fmt.Errorf("COPY failed: %w", err)
			}

		case "USER":
			s.currentUser = instr.Args[0]
			fmt.Printf("Switching build user to %s\n", s.currentUser)

		case "WORKDIR":
			if len(instr.Args) > 0 {
				currentWorkdir = instr.Args[0]
				workdirPath := path.Join(rootfsDir, strings.TrimPrefix(currentWorkdir, "/"))
				_ = s.wslClient.RunDistroCommand("mkdir", "-p", workdirPath)
			}
		}

		// Save Cache after execution
		// Optimization: Skip caching for non-RUN steps UNLESS it's the last step.
		isLastStep := (i == len(df.Instructions)-1)
		if !isSkippable || isLastStep {
			fmt.Printf("Checkpointing state (Step %d/%d)...\n", i+1, len(df.Instructions))
			stepHash := stepHashes[i]
			if err := s.SaveCache(stepHash, rootfsDir); err != nil {
				fmt.Printf("Warning: Failed to save cache for step %d: %v\n", i, err)
			}
		} else {
			fmt.Println("Lightweight step, skipping intermediate checkpoint to save time.")
		}
	}

	// 6. Final Save
	outputTarWsl := path.Join(GetWslImagesDir(), imageName+".tar.gz")

	if allCached {
		fmt.Printf("Shortcut: Mapping last cache layer to final image...\n")
		lastHash := stepHashes[lastHitIndex]
		cacheFile := path.Join(GetWslCacheDir(), lastHash+".tar.gz")
		if err := s.wslClient.RunDistroCommand("cp", cacheFile, outputTarWsl); err != nil {
			return "", fmt.Errorf("build shortcut failed: %w", err)
		}
	} else {
		fmt.Printf("Saving image to %s (WSL)...\n", outputTarWsl)

		startSave := time.Now()
		// Move pipe INSIDE WSL to prevent CRLF corruption via wsl.exe stdout (v0.7.3)
		saveCmd := s.wslClient.PrepareDistroCommand("sh", "-c", fmt.Sprintf("tar -C '%s' -cf - . | gzip > '%s'", rootfsDir, outputTarWsl))
		if err := saveCmd.Start(); err != nil {
			return "", fmt.Errorf("failed to start save: %w", err)
		}

		// Simple progress (elapsed time) since we can't safely monitor bytes via Go stdout
		done := make(chan error, 1)
		go func() {
			done <- saveCmd.Wait()
		}()

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

	Loop:
		for {
			select {
			case err := <-done:
				if err != nil {
					return "", fmt.Errorf("save failed: %w", err)
				}
				break Loop
			case <-ticker.C:
				fmt.Printf("\x1b[2K\rSaving image... (%ds elapsed)", int(time.Since(startSave).Seconds()))
			}
		}
		fmt.Printf("\x1b[2K\rSaving image... done. (%s)\n", time.Since(startSave).Round(time.Second))
	}

	// 7. Save Image Metadata
	// Get CMD if any
	var finalCmd []string
	for _, instr := range df.Instructions {
		if instr.Type == "CMD" {
			// Re-parse CMD form (simple logic matches cmd_run.go)
			args := instr.Raw
			if strings.HasPrefix(args, "[") && strings.HasSuffix(args, "]") {
				trimmed := strings.Trim(args, "[]")
				parts := strings.Split(trimmed, ",")
				for i, p := range parts {
					parts[i] = strings.Trim(strings.TrimSpace(p), "\"")
				}
				finalCmd = parts
			} else {
				finalCmd = []string{"sh", "-c", args}
			}
		}
	}

	metaData := ImageMetadata{
		User:    s.currentUser,
		Workdir: currentWorkdir,
		Env:     envMap,
		Command: finalCmd,
	}
	metaJSON, _ := json.MarshalIndent(metaData, "", "  ")
	metaFileWsl := path.Join(GetWslImagesDir(), imageName+".json")
	_ = s.wslClient.RunDistroCommandWithInput(string(metaJSON), "sh", "-c", fmt.Sprintf("cat > '%s'", metaFileWsl))

	fmt.Printf("\nSuccessfully built image '%s'\n", imageName)
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

	// Write using cat. Use wsl.exe and single quotes for path.
	cmd := exec.Command("wsl.exe", "-d", s.wslClient.DistroName, "sh", "-c", fmt.Sprintf("cat > '%s'", scriptDstPath))
	cmd.Stdin = strings.NewReader(scriptContent)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to write build script to %s: %w", scriptDstPath, err)
	}

	// Make executable
	if err := s.wslClient.RunDistroCommand("chmod", "+x", scriptDstPath); err != nil {
		return fmt.Errorf("failed to chmod script: %w", err)
	}

	// Use su if a non-root user is requested
	execCmd := fmt.Sprintf("/tmp/%s", scriptName)
	if s.currentUser != "" && s.currentUser != "root" {
		execCmd = fmt.Sprintf("su %s -c \"/tmp/%s\"", s.currentUser, scriptName)
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
			"chroot %s %s; "+
			"RET=$?; umount %s/proc %s/sys; exit $RET",
		rootfsDir, rootfsDir, rootfsDir, rootfsDir, currentWorkdir,
		rootfsDir, rootfsDir,
		rootfsDir, rootfsDir, rootfsDir, rootfsDir, // for rm -f
		rootfsDir, rootfsDir, rootfsDir, rootfsDir, // for mknod
		rootfsDir, // for mkdir -p %s/etc
		rootfsDir, // for cat /etc/resolv.conf > ...
		rootfsDir, execCmd,
		rootfsDir, rootfsDir,
	)

	return s.wslClient.RunDistroCommand("unshare", "--mount", "sh", "-c", fullCmd)
}

func (s *WSLImageService) executeBuildCopy(ctxDir, srcArg, destArg, rootfsDir, currentWorkdir string) error {
	src := filepath.Join(ctxDir, srcArg)
	dest := path.Join(rootfsDir, strings.TrimPrefix(path.Join(currentWorkdir, destArg), "/"))

	// Load ignore patterns from .plxignore
	ignorePatterns := make(map[string]bool)
	ignoreFile := filepath.Join(ctxDir, ".plxignore")
	if data, err := os.ReadFile(ignoreFile); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				ignorePatterns[line] = true
			}
		}
	}
	ignorePatterns[".git"] = true

	// Count files for progress visibility, respecting ignore patterns
	fileCount := 0
	_ = filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		relPath, _ := filepath.Rel(src, p)
		if relPath != "." && (ignorePatterns[relPath] || ignorePatterns[filepath.Base(p)]) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			fileCount++
		}
		return nil
	})

	fmt.Printf("STEP: COPY %s to %s (%d files detected after filtering)\n", srcArg, destArg, fileCount)

	srcWsl, err := wsl.WindowsToWslPath(src)
	if err != nil {
		return fmt.Errorf("failed to convert src path: %w", err)
	}

	// ホストからrootfs内へコピー
	// .plxignore にあるディレクトリを除外するために tar を使用する
	fmt.Printf("Copying files from Windows to Linux... ")

	excludeArgs := ""
	for pattern := range ignorePatterns {
		excludeArgs += fmt.Sprintf("--exclude=%q ", pattern)
	}

	// Create parent dir and copy using tar to honor excludes
	cmd := fmt.Sprintf("mkdir -p %s && tar -C %s %s -cf - . | tar -C %s -xf -", dest, srcWsl, excludeArgs, dest)
	err = s.wslClient.RunDistroCommand("sh", "-c", cmd)
	if err == nil {
		fmt.Println("done.")
	} else {
		fmt.Println("failed.")
	}
	return err
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

	fmt.Printf("Restoring state from cache...\n")
	s.wslClient.RunDistroCommand("rm", "-rf", rootfs+"/*")

	startRest := time.Now()
	// Use native tar within WSL for speed (v0.7.5)
	restCmd := s.wslClient.PrepareDistroCommand("tar", "-xzf", cacheFile, "-C", rootfs)
	if err := restCmd.Start(); err != nil {
		return false, fmt.Errorf("failed to start restoration: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- restCmd.Wait()
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

LoopRest:
	for {
		select {
		case err := <-done:
			if err != nil {
				return false, fmt.Errorf("restoration failed: %w", err)
			}
			break LoopRest
		case <-ticker.C:
			fmt.Printf("\x1b[2K\rRestoring... (%ds elapsed)", int(time.Since(startRest).Seconds()))
		}
	}
	return true, nil
}

func (s *WSLImageService) Diff(image1, image2 string) (string, error) {
	imagesDir := GetWslImagesDir()
	path1 := path.Join(imagesDir, image1+".tar.gz")
	path2 := path.Join(imagesDir, image2+".tar.gz")

	// Ensure both images exist
	if err := s.wslClient.RunDistroCommand("test", "-f", path1); err != nil {
		return "", fmt.Errorf("image '%s' not found", image1)
	}
	if err := s.wslClient.RunDistroCommand("test", "-f", path2); err != nil {
		return "", fmt.Errorf("image '%s' not found", image2)
	}

	fmt.Printf("Calculating diff between %s and %s...\n", image1, image2)

	// Get file lists using native tar for speed
	list1, err := s.wslClient.RunDistroCommandOutput("tar", "-ztf", path1)
	if err != nil {
		return "", fmt.Errorf("failed to list files in %s: %w", image1, err)
	}
	list2, err := s.wslClient.RunDistroCommandOutput("tar", "-ztf", path2)
	if err != nil {
		return "", fmt.Errorf("failed to list files in %s: %w", image2, err)
	}

	files1 := make(map[string]bool)
	for _, f := range strings.Split(list1, "\n") {
		f = strings.TrimSpace(f)
		if f != "" {
			files1[f] = true
		}
	}

	files2 := make(map[string]bool)
	for _, f := range strings.Split(list2, "\n") {
		f = strings.TrimSpace(f)
		if f != "" {
			files2[f] = true
		}
	}

	var added, removed []string
	for f := range files2 {
		if !files1[f] {
			added = append(added, f)
		}
	}
	for f := range files1 {
		if !files2[f] {
			removed = append(removed, f)
		}
	}

	// Sort for stable output
	sortStrings(added)
	sortStrings(removed)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Image Diff: %s -> %s\n", image1, image2))
	sb.WriteString(strings.Repeat("-", 40) + "\n")

	if len(added) > 0 {
		sb.WriteString(fmt.Sprintf("ADDED (%d files):\n", len(added)))
		for i, f := range added {
			if i > 20 {
				sb.WriteString("  ...\n")
				break
			}
			sb.WriteString(fmt.Sprintf("  + %s\n", f))
		}
	}

	if len(removed) > 0 {
		sb.WriteString(fmt.Sprintf("\nREMOVED (%d files):\n", len(removed)))
		for i, f := range removed {
			if i > 20 {
				sb.WriteString("  ...\n")
				break
			}
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}

	if len(added) == 0 && len(removed) == 0 {
		sb.WriteString("No changes detected (identical file lists).\n")
	}

	return sb.String(), nil
}

// Internal helper for stable output
func sortStrings(s []string) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[i] > s[j] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

// SaveCache checkpoints the current rootfs state to WSL cache
func (s *WSLImageService) SaveCache(hash string, rootfs string) error {
	cacheDir := GetWslCacheDir()
	cacheFile := path.Join(cacheDir, hash+".tar.gz")

	// Ensure cache dir exists
	if err := s.wslClient.RunDistroCommand("mkdir", "-p", cacheDir); err != nil {
		return err
	}

	fmt.Printf("Saving checkpoint...\n")

	startSave := time.Now()
	// Move pipe INSIDE WSL to prevent CRLF corruption (v0.7.3)
	saveCmd := s.wslClient.PrepareDistroCommand("sh", "-c", fmt.Sprintf("tar -C '%s' -cf - . | gzip > '%s'", rootfs, cacheFile))
	if err := saveCmd.Start(); err != nil {
		return fmt.Errorf("failed to start save: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- saveCmd.Wait()
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

LoopSave:
	for {
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("save failed: %w", err)
			}
			break LoopSave
		case <-ticker.C:
			fmt.Printf("\x1b[2K\rSaving checkpoint... (%ds elapsed)", int(time.Since(startSave).Seconds()))
		}
	}
	fmt.Printf("\x1b[2K\rSaving checkpoint... done. (%s)\n", time.Since(startSave).Round(time.Second))
	fmt.Printf("\nSaving checkpoint... done.\n")
	return nil
}

func (s *WSLImageService) ExportDiff(baseImage, targetImage, outputPath string) error {
	imagesDir := GetWslImagesDir()
	path1 := path.Join(imagesDir, baseImage+".tar.gz")
	path2 := path.Join(imagesDir, targetImage+".tar.gz")

	// Ensure both images exist
	if err := s.wslClient.RunDistroCommand("test", "-f", path1); err != nil {
		return fmt.Errorf("base image '%s' not found", baseImage)
	}
	if err := s.wslClient.RunDistroCommand("test", "-f", path2); err != nil {
		return fmt.Errorf("target image '%s' not found", targetImage)
	}

	// Get added files
	list1, err := s.wslClient.RunDistroCommandOutput("tar", "-ztf", path1)
	if err != nil {
		return fmt.Errorf("failed to list files in base image %s: %w", baseImage, err)
	}
	list2, err := s.wslClient.RunDistroCommandOutput("tar", "-ztf", path2)
	if err != nil {
		return fmt.Errorf("failed to list files in target image %s: %w", targetImage, err)
	}

	files1 := make(map[string]bool)
	for _, f := range strings.Split(list1, "\n") {
		f = strings.TrimSpace(f)
		if f != "" {
			files1[f] = true
		}
	}

	var added []string
	for _, f := range strings.Split(list2, "\n") {
		f = strings.TrimSpace(f)
		if f != "" && !files1[f] {
			added = append(added, f)
		}
	}

	if len(added) == 0 {
		return fmt.Errorf("no differences found between images")
	}

	fmt.Printf("Packaging %d new/modified files from %s...\n", len(added), targetImage)

	// Create a temporary file list in WSL
	tmpFileList := "/tmp/plx_added_files.txt"
	input := strings.Join(added, "\n")
	if err := s.wslClient.RunDistroCommandWithInput(input, "sh", "-c", "cat > "+tmpFileList); err != nil {
		return fmt.Errorf("failed to create temporary file list in WSL: %w", err)
	}

	// Create delta tarball inside WSL
	tmpDeltaTar := "/tmp/plx_delta.tar.gz"
	workspace := "/tmp/plx_export_workspace"
	s.wslClient.RunDistroCommand("rm", "-rf", workspace)
	s.wslClient.RunDistroCommand("mkdir", "-p", workspace)

	fmt.Println("Extracting delta files...")
	extractCmd := fmt.Sprintf("tar -C %s -xzf %s -T %s", workspace, path2, tmpFileList)
	if err := s.wslClient.RunDistroCommand("sh", "-c", extractCmd); err != nil {
		return fmt.Errorf("failed to extract delta files: %w", err)
	}

	fmt.Println("Compressing delta package...")
	compressCmd := fmt.Sprintf("tar -C %s -czf %s .", workspace, tmpDeltaTar)
	if err := s.wslClient.RunDistroCommand("sh", "-c", compressCmd); err != nil {
		return fmt.Errorf("failed to compress delta package: %w", err)
	}

	// Copy out to Windows host
	wslWinPath, _ := wsl.WindowsToWslPath(outputPath)
	if err := s.wslClient.RunDistroCommand("cp", tmpDeltaTar, wslWinPath); err != nil {
		return fmt.Errorf("failed to copy delta package to host path %s: %w", outputPath, err)
	}

	// Cleanup
	s.wslClient.RunDistroCommand("rm", "-rf", workspace, tmpFileList, tmpDeltaTar)

	fmt.Printf("Successfully exported build package to: %s\n", outputPath)
	return nil
}
