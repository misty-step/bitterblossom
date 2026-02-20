package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestTokenExchangeErrUnauthorizedHint(t *testing.T) {
	t.Parallel()

	err := tokenExchangeErr(fmt.Errorf("unauthorized"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "FLY_API_TOKEN may be expired") {
		t.Errorf("error = %q, want hint about expired FLY_API_TOKEN", msg)
	}
	if !strings.Contains(msg, "fly tokens create") {
		t.Errorf("error = %q, want 'fly tokens create' hint", msg)
	}
	if strings.Contains(msg, "set SPRITES_ORG") {
		t.Errorf("error = %q, should not contain SPRITES_ORG hint for unauthorized", msg)
	}
}

func TestTokenExchangeErrOtherErrorOrgHint(t *testing.T) {
	t.Parallel()

	err := tokenExchangeErr(fmt.Errorf("connection refused"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if strings.Contains(msg, "FLY_API_TOKEN may be expired") {
		t.Errorf("error = %q, should not contain expired hint for non-auth error", msg)
	}
	if !strings.Contains(msg, "set SPRITES_ORG") {
		t.Errorf("error = %q, want SPRITES_ORG hint for generic error", msg)
	}
}
