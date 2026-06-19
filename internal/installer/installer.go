// Package installer pulls and verifies Docker images for managed components.
package installer

import (
	"context"
	"fmt"

	"routestack-agent/internal/components"
	"routestack-agent/internal/docker"
)

// ImageClient is the subset of the Docker client used by Installer.
type ImageClient interface {
	Pull(ctx context.Context, image string) error
	InspectImage(ctx context.Context, image string) (*docker.ImageInfo, error)
}

// Installer pulls component images and verifies their SHA-256 digests.
type Installer struct {
	client ImageClient
}

// NewInstaller creates an installer bound to the given Docker image client.
func NewInstaller(client ImageClient) *Installer {
	return &Installer{client: client}
}

// Install pulls the component's image and validates its digest.
// It returns an error if the image cannot be pulled or the digest mismatch.
func (i *Installer) Install(ctx context.Context, c components.Component) error {
	if err := c.Validate(); err != nil {
		return fmt.Errorf("invalid component: %w", err)
	}

	image := c.FullImage()

	if err := i.client.Pull(ctx, image); err != nil {
		return fmt.Errorf("pull %s: %w", image, err)
	}

	info, err := i.client.InspectImage(ctx, image)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", image, err)
	}

	if !components.MatchDigest(c.ExpectedDigest, info.RepoDigests) {
		return fmt.Errorf("digest mismatch for %s: expected %s, got %v", image, c.ExpectedDigest, info.RepoDigests)
	}

	return nil
}
