package sprite

import (
	"reflect"
	"testing"
)

func TestWithOrgArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		base []string
		org  string
		want []string
	}{
		{
			name: "no org keeps args",
			base: []string{"list"},
			org:  "",
			want: []string{"list"},
		},
		{
			name: "appends org at end when no separator",
			base: []string{"api", "/orgs"},
			org:  "misty-step",
			want: []string{"api", "/orgs", "-o", "misty-step"},
		},
		{
			name: "inserts org before separator",
			base: []string{"exec", "-s", "bramble", "--", "bash", "-ceu", "echo ok"},
			org:  "misty-step",
			want: []string{"exec", "-s", "bramble", "-o", "misty-step", "--", "bash", "-ceu", "echo ok"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := withOrgArgs(tc.base, tc.org)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("withOrgArgs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCreateArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		org  string
		want []string
	}{
		{
			name: "with org",
			org:  "misty-step",
			want: []string{"create", "-skip-console", "-o", "misty-step", "bramble"},
		},
		{
			name: "without org",
			org:  "",
			want: []string{"create", "-skip-console", "bramble"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := createArgs("bramble", tc.org)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("createArgs() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDestroyArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		org  string
		want []string
	}{
		{
			name: "with org",
			org:  "misty-step",
			want: []string{"destroy", "-force", "-o", "misty-step", "bramble"},
		},
		{
			name: "without org",
			org:  "",
			want: []string{"destroy", "-force", "bramble"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := destroyArgs("bramble", tc.org)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("destroyArgs() = %v, want %v", got, tc.want)
			}
		})
	}
}
