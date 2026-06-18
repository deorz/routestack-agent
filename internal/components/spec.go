// Package components defines the agent's component catalog.
//
// A component is a Docker image that the agent can pull, verify, and
// reference when creating managed service containers.
package components

import (
	"errors"
	"fmt"
	"strings"
)

// Component describes a managed Docker image.
type Component struct {
	// Name is the logical component name (e.g. "amnezia-awg2", "xray", "telemt").
	Name string

	// Version is the component version (e.g. "v2.0.1").
	Version string

	// Image is the Docker image reference, including registry and tag.
	Image string

	// ExpectedDigest is the SHA-256 digest the pulled image must match.
	// Format: "sha256:<hex64>".
	ExpectedDigest string
}

// Validate returns an error if the component spec is incomplete or malformed.
func (c Component) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("component name is required")
	}
	if strings.TrimSpace(c.Version) == "" {
		return errors.New("component version is required")
	}
	if strings.TrimSpace(c.Image) == "" {
		return errors.New("component image is required")
	}
	if !strings.Contains(c.Image, ":") {
		return errors.New("component image must include a tag or digest")
	}
	return ValidateDigest(c.ExpectedDigest)
}

// FullImage returns the image reference with version tag.
// If the image already contains a digest, the original value is returned.
func (c Component) FullImage() string {
	if strings.Contains(c.Image, "@sha256:") {
		return c.Image
	}
	// If image already has a tag, trust it; otherwise append version.
	if strings.Contains(c.Image, ":") {
		return c.Image
	}
	return fmt.Sprintf("%s:%s", c.Image, c.Version)
}

// ValidateDigest checks that s is a valid SHA-256 digest.
func ValidateDigest(s string) error {
	const prefix = "sha256:"
	if !strings.HasPrefix(s, prefix) {
		return errors.New("expected digest must have sha256: prefix")
	}
	hex := strings.TrimPrefix(s, prefix)
	if len(hex) != 64 {
		return errors.New("expected digest must have 64 hex characters")
	}
	for _, r := range hex {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return errors.New("expected digest must be lowercase hex")
		}
	}
	return nil
}

// MatchDigest reports whether the pulled image digest matches the expected value.
// It accepts RepoDigests entries from docker.Client.InspectImage.
func MatchDigest(expected string, repoDigests []string) bool {
	for _, rd := range repoDigests {
		if strings.HasSuffix(rd, expected) {
			return true
		}
		if rd == expected {
			return true
		}
	}
	return false
}
