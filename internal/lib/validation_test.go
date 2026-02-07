package lib

import "testing"

func TestValidateSpriteName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid_simple", input: "bramble"},
		{name: "valid_hyphen", input: "sprite-1"},
		{name: "starts_with_number", input: "1sprite", wantErr: true},
		{name: "uppercase", input: "Bramble", wantErr: true},
		{name: "underscore", input: "sprite_name", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSpriteName(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestValidateRepoRef(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "org_repo", input: "misty-step/cerberus"},
		{name: "https_url", input: "https://github.com/misty-step/cerberus"},
		{name: "http_url_rejected", input: "http://github.com/foo/bar", wantErr: true},
		{name: "invalid", input: "foo", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRepoRef(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}
