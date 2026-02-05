package container

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseDockerfile parses a Dockerfile from the given path
func ParseDockerfile(path string) (*Dockerfile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	df := &Dockerfile{
		Env: make(map[string]string),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Dockerfile instructions are case-insensitive by convention
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		instruction := strings.ToUpper(parts[0])
		args := line[len(parts[0]):]
		args = strings.TrimSpace(args)

		switch instruction {
		case "FROM":
			df.Base = args
		case "RUN":
			df.Run = append(df.Run, args)
		case "ENV":
			// Handle ENV KEY VALUE and ENV KEY=VALUE
			if strings.Contains(args, "=") {
				kv := strings.SplitN(args, "=", 2)
				df.Env[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			} else {
				kv := strings.Fields(args)
				if len(kv) >= 2 {
					df.Env[kv[0]] = strings.Join(kv[1:], " ")
				}
			}
		case "EXPOSE":
			// Handle multiple ports like EXPOSE 80 443
			ports := strings.Fields(args)
			for _, pStr := range ports {
				if p, err := strconv.Atoi(pStr); err == nil {
					df.Expose = append(df.Expose, p)
				}
			}
		case "CMD":
			// Simple shell/exec form detection
			if strings.HasPrefix(args, "[") && strings.HasSuffix(args, "]") {
				trimmed := strings.Trim(args, "[]")
				parts := strings.Split(trimmed, ",")
				for i, p := range parts {
					parts[i] = strings.Trim(strings.TrimSpace(p), "\"")
				}
				df.Cmd = parts
			} else {
				df.Cmd = []string{"sh", "-c", args}
			}
		case "WORKDIR":
			df.Workdir = args
		case "COPY":
			copyParts := strings.Fields(args)
			if len(copyParts) >= 2 {
				dest := copyParts[len(copyParts)-1]
				src := strings.Join(copyParts[:len(copyParts)-1], " ")
				df.Copy = append(df.Copy, [2]string{src, dest})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if df.Base == "" {
		return nil, fmt.Errorf("Dockerfile must start with FROM")
	}

	return df, nil
}
