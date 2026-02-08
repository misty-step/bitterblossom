package lifecycle

import "testing"

func TestValidateSpriteName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "simple valid", input: "bramble"},
		{name: "hyphen valid", input: "thorn-beta"},
		{name: "starts number", input: "1thorn", wantErr: true},
		{name: "uppercase", input: "Thorn", wantErr: true},
		{name: "special chars", input: "thorn_beta", wantErr: true},
		{name: "empty", input: "", wantErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateSpriteName(test.input)
			if test.wantErr && err == nil {
				t.Fatalf("ValidateSpriteName(%q) expected error", test.input)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("ValidateSpriteName(%q) unexpected error: %v", test.input, err)
			}
		})
	}
}
