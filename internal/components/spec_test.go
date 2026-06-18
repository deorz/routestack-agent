package components

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	valid := Component{
		Name:           "telemt",
		Version:        "v1.2.3",
		Image:          "ghcr.io/telemt/telemt:v1.2.3",
		ExpectedDigest: "sha256:" + "0123456789abcdef" + "0123456789abcdef" + "0123456789abcdef" + "0123456789abcdef",
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid component rejected: %v", err)
	}

	invalid := []struct {
		name string
		comp Component
		want string
	}{
		{
			name: "missing name",
			comp: Component{Version: "v1", Image: "x:y", ExpectedDigest: valid.ExpectedDigest},
			want: "name is required",
		},
		{
			name: "missing version",
			comp: Component{Name: "x", Image: "x:y", ExpectedDigest: valid.ExpectedDigest},
			want: "version is required",
		},
		{
			name: "missing image",
			comp: Component{Name: "x", Version: "v1", ExpectedDigest: valid.ExpectedDigest},
			want: "image is required",
		},
		{
			name: "image without tag",
			comp: Component{Name: "x", Version: "v1", Image: "x", ExpectedDigest: valid.ExpectedDigest},
			want: "tag",
		},
		{
			name: "bad digest prefix",
			comp: Component{Name: "x", Version: "v1", Image: "x:y", ExpectedDigest: "abc123"},
			want: "sha256",
		},
		{
			name: "digest too short",
			comp: Component{Name: "x", Version: "v1", Image: "x:y", ExpectedDigest: "sha256:abc"},
			want: "64",
		},
		{
			name: "digest uppercase",
			comp: Component{Name: "x", Version: "v1", Image: "x:y", ExpectedDigest: "sha256:" + "0123456789ABCDEF" + "0123456789abcdef" + "0123456789abcdef" + "0123456789abcdef"},
			want: "lowercase",
		},
	}

	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.comp.Validate()
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestFullImage(t *testing.T) {
	c := Component{Image: "alpine", Version: "3.18"}
	if got := c.FullImage(); got != "alpine:3.18" {
		t.Errorf("FullImage() = %q, want alpine:3.18", got)
	}

	c2 := Component{Image: "alpine:3.19", Version: "3.18"}
	if got := c2.FullImage(); got != "alpine:3.19" {
		t.Errorf("FullImage() = %q, want alpine:3.19", got)
	}

	c3 := Component{Image: "alpine@sha256:deadbeef", Version: "3.18"}
	if got := c3.FullImage(); got != "alpine@sha256:deadbeef" {
		t.Errorf("FullImage() = %q, want alpine@sha256:deadbeef", got)
	}
}

func TestMatchDigest(t *testing.T) {
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	cases := []struct {
		digests []string
		want    bool
	}{
		{[]string{"alpine@" + digest}, true},
		{[]string{digest}, true},
		{[]string{"alpine@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"}, false},
		{nil, false},
	}
	for i, tc := range cases {
		if got := MatchDigest(digest, tc.digests); got != tc.want {
			t.Errorf("case %d: MatchDigest = %v, want %v", i, got, tc.want)
		}
	}
}
