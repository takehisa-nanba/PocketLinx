package container

import (
	"bufio"
	"fmt"
	"os"
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
		Instructions: make([]Instruction, 0),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Handle line continuation
		for strings.HasSuffix(line, "\\") && scanner.Scan() {
			line = strings.TrimSuffix(line, "\\") + " " + strings.TrimSpace(scanner.Text())
		}

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
		default:
			// Parse specific args for better structure if needed, but store everything in order
			parsedArgs := []string{}

			switch instruction {
			case "ENV":
				// Handle ENV KEY VALUE and ENV KEY=VALUE
				if strings.Contains(args, "=") {
					kv := strings.SplitN(args, "=", 2)
					parsedArgs = append(parsedArgs, strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]))
				} else {
					kv := strings.Fields(args)
					if len(kv) >= 2 {
						parsedArgs = append(parsedArgs, kv[0], strings.Join(kv[1:], " "))
					}
				}
			case "COPY":
				copyParts := strings.Fields(args)
				if len(copyParts) >= 2 {
					dest := copyParts[len(copyParts)-1]
					src := strings.Join(copyParts[:len(copyParts)-1], " ")
					parsedArgs = append(parsedArgs, src, dest)
				}
			case "CMD":
				// Simple shell/exec form detection
				if strings.HasPrefix(args, "[") && strings.HasSuffix(args, "]") {
					trimmed := strings.Trim(args, "[]")
					parts := strings.Split(trimmed, ",")
					for i, p := range parts {
						parts[i] = strings.Trim(strings.TrimSpace(p), "\"")
					}
					parsedArgs = parts
				} else {
					parsedArgs = []string{"sh", "-c", args}
				}
			case "WORKDIR":
				parsedArgs = []string{args}
			case "RUN":
				parsedArgs = []string{args}
			case "EXPOSE":
				parsedArgs = strings.Fields(args)
			default:
				// Generic fallback
				parsedArgs = []string{args}
			}

			df.Instructions = append(df.Instructions, Instruction{
				Type: instruction,
				Args: parsedArgs,
				Raw:  args,
			})
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
