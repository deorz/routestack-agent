package installer

import (
	"context"
	"errors"
	"testing"

	"routestack-agent/internal/components"
	"routestack-agent/internal/docker"
)

type mockImageClient struct {
	pulled     []string
	info       *docker.ImageInfo
	pullErr    error
	inspectErr error
}

func (m *mockImageClient) Pull(_ context.Context, image string) error {
	m.pulled = append(m.pulled, image)
	return m.pullErr
}

func (m *mockImageClient) InspectImage(_ context.Context, _ string) (*docker.ImageInfo, error) {
	return m.info, m.inspectErr
}

func TestInstallerInstall(t *testing.T) {
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	client := &mockImageClient{
		info: &docker.ImageInfo{RepoDigests: []string{"ghcr.io/telemt/telemt:v1.2.3@" + digest}},
	}
	inst := NewInstaller(client)

	c := components.Component{
		Name:           "telemt",
		Version:        "v1.2.3",
		Image:          "ghcr.io/telemt/telemt:v1.2.3",
		ExpectedDigest: digest,
	}

	if err := inst.Install(context.Background(), c); err != nil {
		t.Fatalf("Install failed: %v", err)
	}
	if len(client.pulled) != 1 || client.pulled[0] != c.Image {
		t.Errorf("pulled = %v, want [%s]", client.pulled, c.Image)
	}
}

func TestInstallerDigestMismatch(t *testing.T) {
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	client := &mockImageClient{
		info: &docker.ImageInfo{RepoDigests: []string{"ghcr.io/telemt/telemt:v1.2.3@sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"}},
	}
	inst := NewInstaller(client)

	c := components.Component{
		Name:           "telemt",
		Version:        "v1.2.3",
		Image:          "ghcr.io/telemt/telemt:v1.2.3",
		ExpectedDigest: digest,
	}

	err := inst.Install(context.Background(), c)
	if err == nil {
		t.Fatal("expected digest mismatch error")
	}
	if !errors.Is(err, errors.New("digest mismatch")) {
		// Error string check is acceptable for constructed error.
		if err.Error()[:15] != "digest mismatch" {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

func TestInstallerPullError(t *testing.T) {
	client := &mockImageClient{pullErr: errors.New("network error")}
	inst := NewInstaller(client)

	c := components.Component{
		Name:           "telemt",
		Version:        "v1.2.3",
		Image:          "ghcr.io/telemt/telemt:v1.2.3",
		ExpectedDigest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}

	if err := inst.Install(context.Background(), c); err == nil {
		t.Fatal("expected pull error")
	}
}
