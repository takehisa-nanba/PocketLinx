//go:build linux

package container

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// LinuxImageService implements ImageService for native Linux
type LinuxImageService struct {
	rootDir string
}

func NewLinuxImageService(rootDir string) *LinuxImageService {
	return &LinuxImageService{rootDir: rootDir}
}

func (s *LinuxImageService) Pull(image string) error {
	url, ok := SupportedImages[image]
	if !ok {
		return fmt.Errorf("image '%s' is not supported", image)
	}

	targetFile := filepath.Join(s.rootDir, "images", image+".tar.gz")
	if _, err := os.Stat(targetFile); err == nil {
		fmt.Printf("Image '%s' already exists.\n", image)
		return nil
	}

	fmt.Printf("Pulling image '%s' from %s...\n", image, url)
	if err := downloadFile(url, targetFile); err != nil {
		return fmt.Errorf("error executing download: %w", err)
	}

	return nil
}

func (s *LinuxImageService) Images() ([]string, error) {
	imagesDir := filepath.Join(s.rootDir, "images")
	files, err := os.ReadDir(imagesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var images []string
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".tar.gz") {
			name := strings.TrimSuffix(f.Name(), ".tar.gz")
			images = append(images, name)
		}
	}
	return images, nil
}

func (s *LinuxImageService) Prune() error {
	return os.RemoveAll(filepath.Join(s.rootDir, "cache"))
}

func (s *LinuxImageService) Build(ctxDir string, dockerfile string, tag string) (string, error) {
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	dockerfilePath := filepath.Join(ctxDir, dockerfile)
	df, err := ParseDockerfile(dockerfilePath)
	if err != nil {
		return "", fmt.Errorf("failed to parse Dockerfile: %w", err)
	}

	imageName := tag
	if imageName == "" {
		imageName = strings.ToLower(filepath.Base(ctxDir))
		if imageName == "." {
			abs, _ := filepath.Abs(ctxDir)
			imageName = strings.ToLower(filepath.Base(abs))
		}
	}

	buildId := fmt.Sprintf("build-%d", os.Getpid())
	buildDir := filepath.Join(s.rootDir, "builds", buildId)
	rootfsDir := filepath.Join(buildDir, "rootfs")
	defer os.RemoveAll(buildDir)

	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		return "", err
	}

	// Base image
	if err := s.Pull(df.Base); err != nil {
		return "", err
	}
	baseTar := filepath.Join(s.rootDir, "images", df.Base+".tar.gz")
	exec.Command("tar", "-xf", baseTar, "-C", rootfsDir).Run()

	// 2. Build Steps
	envPrefix := ""

	for _, instr := range df.Instructions {
		switch instr.Type {
		case "ENV":
			for i := 0; i < len(instr.Args); i += 2 {
				k := instr.Args[i]
				v := ""
				if i+1 < len(instr.Args) {
					v = instr.Args[i+1]
				}
				envPrefix += fmt.Sprintf("export %s=%q; ", k, v)
			}
		case "WORKDIR":
			// Skip for now in simplified linux build
		case "RUN":
			runCmd := instr.Raw
			fmt.Printf("STEP: RUN %s\n", runCmd)

			resolvConfPath := filepath.Join(rootfsDir, "etc/resolv.conf")
			_ = os.MkdirAll(filepath.Dir(resolvConfPath), 0755)
			exec.Command("cp", "/etc/resolv.conf", resolvConfPath).Run()

			shimPath := "/usr/local/bin/container-shim"
			if _, err := os.Stat(shimPath); os.IsNotExist(err) {
				return "", fmt.Errorf("container-shim not found at %s. Please run 'plx setup' first", shimPath)
			}

			fullUserCmd := fmt.Sprintf("%s%s", envPrefix, runCmd)
			cmdArgs := []string{"--mount", "--pid", "--fork", "--uts", "--propagation", "unchanged"}
			cmdArgs = append(cmdArgs, shimPath, rootfsDir, "none", "/bin/sh", "-c", fullUserCmd)

			runExec := exec.Command("unshare", cmdArgs...)
			runExec.Stdin = os.Stdin
			runExec.Stdout = os.Stdout
			runExec.Stderr = os.Stderr

			if err := runExec.Run(); err != nil {
				return "", fmt.Errorf("RUN failed: %w", err)
			}

		case "COPY":
			src := filepath.Join(ctxDir, instr.Args[0])
			dst := filepath.Join(rootfsDir, instr.Args[1])
			_ = os.MkdirAll(filepath.Dir(dst), 0755)
			if err := exec.Command("cp", "-r", src, dst).Run(); err != nil {
				return "", fmt.Errorf("COPY failed: %w", err)
			}
		}
	}

	// 4. Save
	if err := os.MkdirAll(filepath.Join(s.rootDir, "images"), 0755); err != nil {
		return "", err
	}
	outTar := filepath.Join(s.rootDir, "images", imageName+".tar.gz")
	fmt.Printf("Saving image to %s...\n", outTar)
	if err := exec.Command("tar", "-czf", outTar, "-C", rootfsDir, ".").Run(); err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	return imageName, nil
}

func (s *LinuxImageService) Diff(image1, image2 string) (string, error) {
	return "", fmt.Errorf("diff not implemented for native linux yet")
}

func (s *LinuxImageService) ExportDiff(baseImage, targetImage, outputPath string) error {
	return fmt.Errorf("export-diff not implemented for native linux yet")
}
