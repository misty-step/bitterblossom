package dispatch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// maxSecretFileSize limits individual secret files to 128KB.
	maxSecretFileSize = 128 * 1024
	// maxSecretsFromDir limits the number of files loaded from ~/.secrets.
	maxSecretsFromDir = 64
)

// SecretResolver resolves secret references from various sources (1Password, env vars, files).
type SecretResolver struct {
	// ResolveOP resolves 1Password references (op://vault/item/field).
	// If nil, 1Password resolution will fail.
	ResolveOP func(ref string) (string, error)
}

// SecretSource represents a parsed secret flag (NAME=reference).
type SecretSource struct {
	Name      string
	Reference string
}

// ParseFlag parses a --secret flag in the format NAME=REFERENCE.
// REFERENCE can be:
//   - Direct value: KEY=value
//   - 1Password: KEY=op://vault/item/field
//   - Environment variable: KEY=${env:ENV_VAR_NAME}
//   - File: KEY=${file:/path/to/file}
func (r *SecretResolver) ParseFlag(flag string) (name, reference string, err error) {
	flag = strings.TrimSpace(flag)
	if flag == "" {
		return "", "", fmt.Errorf("empty secret flag")
	}

	parts := strings.SplitN(flag, "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("secret flag must be in format NAME=REFERENCE: %q", flag)
	}

	name = strings.TrimSpace(parts[0])
	reference = strings.TrimSpace(parts[1])

	if name == "" {
		return "", "", fmt.Errorf("secret flag has empty name: %q", flag)
	}
	if reference == "" {
		return "", "", fmt.Errorf("secret flag has empty reference for %q", name)
	}

	return name, reference, nil
}

// Resolve resolves a secret reference to its actual value.
func (r *SecretResolver) Resolve(reference string) (string, error) {
	reference = strings.TrimSpace(reference)

	// 1Password reference: op://vault/item/field
	if strings.HasPrefix(reference, "op://") {
		if r.ResolveOP == nil {
			return "", fmt.Errorf("1Password resolution not available for %q", reference)
		}
		return r.ResolveOP(reference)
	}

	// Environment variable reference: ${env:VAR_NAME}
	if strings.HasPrefix(reference, "${env:") && strings.HasSuffix(reference, "}") {
		varName := reference[len("${env:") : len(reference)-1]
		value, ok := os.LookupEnv(varName)
		if !ok || value == "" {
			return "", fmt.Errorf("environment variable %q is not set or is empty", varName)
		}
		return value, nil
	}

	// File reference: ${file:/path/to/file}
	if strings.HasPrefix(reference, "${file:") && strings.HasSuffix(reference, "}") {
		filePath := reference[len("${file:") : len(reference)-1]
		info, err := os.Stat(filePath)
		if err != nil {
			return "", fmt.Errorf("read secret file %q: %w", filePath, err)
		}
		if info.Size() > maxSecretFileSize {
			return "", fmt.Errorf("secret file %q exceeds maximum size (%d bytes)", filePath, maxSecretFileSize)
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("read secret file %q: %w", filePath, err)
		}
		return strings.TrimSpace(string(content)), nil
	}

	// Default: treat as direct value
	return reference, nil
}

// ResolveAll resolves all secret flags and returns:
//   - resolved: map of secret names to resolved values (for container env vars)
//   - placeholders: map of secret names to placeholder strings (for model visibility)
func (r *SecretResolver) ResolveAll(flags []string) (resolved, placeholders map[string]string, err error) {
	resolved = make(map[string]string, len(flags))
	placeholders = make(map[string]string, len(flags))

	for _, flag := range flags {
		name, reference, err := r.ParseFlag(flag)
		if err != nil {
			return nil, nil, fmt.Errorf("parse secret flag: %w", err)
		}

		value, err := r.Resolve(reference)
		if err != nil {
			return nil, nil, fmt.Errorf("resolve secret %q: %w", name, err)
		}

		resolved[name] = value
		placeholders[name] = fmt.Sprintf("$%s", name)
	}

	return resolved, placeholders, nil
}

// DefaultSecretResolver creates a SecretResolver with default 1Password resolution.
func DefaultSecretResolver() *SecretResolver {
	return &SecretResolver{
		ResolveOP: resolveOnePassword,
	}
}

// resolveOnePassword resolves a 1Password reference using the op CLI.
// Reference format: op://vault/item/field
func resolveOnePassword(ref string) (string, error) {
	// Validate the reference format
	if !strings.HasPrefix(ref, "op://") {
		return "", fmt.Errorf("invalid 1Password reference format: %q (expected op://vault/item/field)", ref)
	}

	// Execute `op read` to resolve the reference
	// The op CLI handles authentication (biometric, CLI key, etc.)
	cmd := exec.Command("op", "read", ref)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("1Password CLI failed: %w (stderr: %s)", err, string(exitErr.Stderr))
		}
		return "", fmt.Errorf("1Password CLI failed: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// LoadSecretsFromDir loads secrets from files in a directory.
// Each file's name becomes the secret name, and its content becomes the value.
func LoadSecretsFromDir(dir string) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Empty if directory doesn't exist
		}
		return nil, fmt.Errorf("read secrets directory: %w", err)
	}

	secrets := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if len(secrets) >= maxSecretsFromDir {
			return nil, fmt.Errorf("too many secret files in %q (max %d)", dir, maxSecretsFromDir)
		}

		path := filepath.Join(dir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("stat secret file %q: %w", entry.Name(), err)
		}
		if info.Size() > maxSecretFileSize {
			return nil, fmt.Errorf("secret file %q exceeds maximum size (%d bytes)", entry.Name(), maxSecretFileSize)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read secret file %q: %w", entry.Name(), err)
		}

		secrets[entry.Name()] = strings.TrimSpace(string(content))
	}

	return secrets, nil
}