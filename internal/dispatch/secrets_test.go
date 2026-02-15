package dispatch

import (
	"errors"
	"os"
	"testing"
)

func TestSecretResolver_Resolve(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantValue string
		wantErr   bool
	}{
		{
			name:      "direct value returns as-is",
			input:     "my-secret-value",
			wantValue: "my-secret-value",
			wantErr:   false,
		},
		{
			name:      "env placeholder with env var set",
			input:     "${env:GITHUB_TOKEN}",
			wantValue: "test-github-token",
			wantErr:   false,
		},
		{
			name:    "env placeholder with env var unset",
			input:   "${env:UNSET_VAR}",
			wantErr: true,
		},
		{
			name:      "file placeholder",
			input:     "${file:/tmp/test-secret.txt}",
			wantValue: "file-secret-content",
			wantErr:   false,
		},
		{
			name:    "file placeholder with missing file",
			input:   "${file:/nonexistent/path}",
			wantErr: true,
		},
		{
			name:      "op placeholder (mocked)",
			input:     "op://vault/item/field",
			wantValue: "op-secret-value",
			wantErr:   false,
		},
	}

	// Set up test env var
	t.Setenv("GITHUB_TOKEN", "test-github-token")

	// Create test file
	tmpFile := t.TempDir() + "/test-secret.txt"
	if err := writeFile(tmpFile, []byte("file-secret-content")); err != nil {
		t.Fatalf("setup: write test file: %v", err)
	}

	resolver := &SecretResolver{
		ResolveOP: func(ref string) (string, error) {
			if ref == "op://vault/item/field" {
				return "op-secret-value", nil
			}
			return "", errors.New("mock: unknown 1Password reference")
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Adjust file path for test
			input := tt.input
			if input == "${file:/tmp/test-secret.txt}" {
				input = "${file:" + tmpFile + "}"
			}

			got, err := resolver.Resolve(input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Resolve() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantValue {
				t.Errorf("Resolve() = %v, want %v", got, tt.wantValue)
			}
		})
	}
}

func TestSecretResolver_ParseFlag(t *testing.T) {
	tests := []struct {
		name     string
		flag     string
		wantName string
		wantRef  string
		wantErr  bool
	}{
		{
			name:     "simple key=value",
			flag:     "GITHUB_TOKEN=ghp_12345",
			wantName: "GITHUB_TOKEN",
			wantRef:  "ghp_12345",
			wantErr:  false,
		},
		{
			name:     "1Password reference",
			flag:     "GITHUB_TOKEN=op://vault/item/field",
			wantName: "GITHUB_TOKEN",
			wantRef:  "op://vault/item/field",
			wantErr:  false,
		},
		{
			name:     "env reference",
			flag:     "API_KEY=${env:API_KEY}",
			wantName: "API_KEY",
			wantRef:  "${env:API_KEY}",
			wantErr:  false,
		},
		{
			name:    "missing equals sign",
			flag:    "GITHUB_TOKEN",
			wantErr: true,
		},
		{
			name:    "empty key",
			flag:    "=value",
			wantErr: true,
		},
		{
			name:    "empty value",
			flag:    "KEY=",
			wantErr: true,
		},
		{
			name:     "value with equals sign",
			flag:     "KEY=value=with=equals",
			wantName: "KEY",
			wantRef:  "value=with=equals",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &SecretResolver{}
			gotName, gotRef, err := resolver.ParseFlag(tt.flag)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFlag() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotName != tt.wantName {
				t.Errorf("ParseFlag() name = %v, want %v", gotName, tt.wantName)
			}
			if gotRef != tt.wantRef {
				t.Errorf("ParseFlag() ref = %v, want %v", gotRef, tt.wantRef)
			}
		})
	}
}

func TestSecretResolver_ResolveAll(t *testing.T) {
	resolver := &SecretResolver{
		ResolveOP: func(ref string) (string, error) {
			if ref == "op://vault/token/field" {
				return "resolved-op-token", nil
			}
			return "", errors.New("unknown ref")
		},
	}

	t.Setenv("DIRECT_TOKEN", "direct-value")

	flags := []string{
		"GITHUB_TOKEN=op://vault/token/field",
		"API_KEY=${env:DIRECT_TOKEN}",
	}

	resolved, placeholders, err := resolver.ResolveAll(flags)
	if err != nil {
		t.Fatalf("ResolveAll() error = %v", err)
	}

	// Check resolved values
	if resolved["GITHUB_TOKEN"] != "resolved-op-token" {
		t.Errorf("GITHUB_TOKEN = %v, want resolved-op-token", resolved["GITHUB_TOKEN"])
	}
	if resolved["API_KEY"] != "direct-value" {
		t.Errorf("API_KEY = %v, want direct-value", resolved["API_KEY"])
	}

	// Check placeholders
	if placeholders["GITHUB_TOKEN"] != "$GITHUB_TOKEN" {
		t.Errorf("GITHUB_TOKEN placeholder = %v, want $GITHUB_TOKEN", placeholders["GITHUB_TOKEN"])
	}
	if placeholders["API_KEY"] != "$API_KEY" {
		t.Errorf("API_KEY placeholder = %v, want $API_KEY", placeholders["API_KEY"])
	}
}

func TestSecretResolver_ResolveAll_Error(t *testing.T) {
	resolver := &SecretResolver{
		ResolveOP: func(ref string) (string, error) {
			return "", errors.New("op not available")
		},
	}

	flags := []string{
		"GITHUB_TOKEN=op://vault/token/field",
	}

	_, _, err := resolver.ResolveAll(flags)
	if err == nil {
		t.Error("ResolveAll() expected error for failed 1Password resolution")
	}
}

func TestLoadSecretsFromDir(t *testing.T) {
	t.Run("nonexistent directory returns nil", func(t *testing.T) {
		secrets, err := LoadSecretsFromDir("/nonexistent/path/that/does/not/exist")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if secrets != nil {
			t.Fatalf("expected nil secrets, got %v", secrets)
		}
	})

	t.Run("regular file (not directory) returns nil", func(t *testing.T) {
		// Create a regular file where a directory is expected
		tmpFile := t.TempDir() + "/not-a-dir"
		if err := os.WriteFile(tmpFile, []byte("I am a file"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		secrets, err := LoadSecretsFromDir(tmpFile)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if secrets != nil {
			t.Fatalf("expected nil secrets, got %v", secrets)
		}
	})

	t.Run("valid directory loads secrets", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(dir+"/API_KEY", []byte("secret-value\n"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(dir+"/DB_PASS", []byte("  db-pass  "), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		// Hidden files should be skipped
		if err := os.WriteFile(dir+"/.hidden", []byte("skip-me"), 0644); err != nil {
			t.Fatalf("setup: %v", err)
		}

		secrets, err := LoadSecretsFromDir(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(secrets) != 2 {
			t.Fatalf("expected 2 secrets, got %d", len(secrets))
		}
		if secrets["API_KEY"] != "secret-value" {
			t.Errorf("API_KEY = %q, want %q", secrets["API_KEY"], "secret-value")
		}
		if secrets["DB_PASS"] != "db-pass" {
			t.Errorf("DB_PASS = %q, want %q", secrets["DB_PASS"], "db-pass")
		}
	})

	t.Run("empty directory returns empty map", func(t *testing.T) {
		dir := t.TempDir()
		secrets, err := LoadSecretsFromDir(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(secrets) != 0 {
			t.Fatalf("expected 0 secrets, got %d", len(secrets))
		}
	})
}

// Helper function to write files for tests
func writeFile(path string, content []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(content)
	return err
}
