package dispatch

import (
	"errors"
	"testing"
)

func TestValidateNoDirectAnthropic_EmptyKey(t *testing.T) {
	env := map[string]string{"ANTHROPIC_API_KEY": ""}
	if err := ValidateNoDirectAnthropic(env, false); err != nil {
		t.Fatalf("expected no error for empty key, got: %v", err)
	}
}

func TestValidateNoDirectAnthropic_ProxyMode(t *testing.T) {
	env := map[string]string{"ANTHROPIC_API_KEY": "proxy-mode"}
	if err := ValidateNoDirectAnthropic(env, false); err != nil {
		t.Fatalf("expected no error for proxy-mode, got: %v", err)
	}
}

func TestValidateNoDirectAnthropic_Unset(t *testing.T) {
	env := map[string]string{}
	if err := ValidateNoDirectAnthropic(env, false); err != nil {
		t.Fatalf("expected no error for unset key, got: %v", err)
	}
}

func TestValidateNoDirectAnthropic_RealKey_Blocked(t *testing.T) {
	env := map[string]string{"ANTHROPIC_API_KEY": "sk-ant-api03-abcdef123456"}
	err := ValidateNoDirectAnthropic(env, false)
	if err == nil {
		t.Fatal("expected error for real sk-ant- key")
	}
	var keyErr *ErrDirectAnthropicKey
	if !errors.As(err, &keyErr) {
		t.Fatalf("expected ErrDirectAnthropicKey, got %T: %v", err, err)
	}
	if keyErr.KeyPrefix != "sk-ant-api03" {
		t.Fatalf("expected prefix 'sk-ant-api03', got %q", keyErr.KeyPrefix)
	}
}

func TestValidateNoDirectAnthropic_RealKey_AllowDirect(t *testing.T) {
	env := map[string]string{"ANTHROPIC_API_KEY": "sk-ant-api03-abcdef123456"}
	if err := ValidateNoDirectAnthropic(env, true); err != nil {
		t.Fatalf("expected no error with allowDirect=true, got: %v", err)
	}
}

func TestValidateNoDirectAnthropic_NonAnthropicKey(t *testing.T) {
	env := map[string]string{"ANTHROPIC_API_KEY": "some-other-value"}
	if err := ValidateNoDirectAnthropic(env, false); err != nil {
		t.Fatalf("expected no error for non-anthropic key, got: %v", err)
	}
}
