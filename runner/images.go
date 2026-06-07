package runner

import (
	"context"
	"os/exec"
	"time"
)

// DefaultImage is the pre-baked stepthrough agent image published to GHCR.
// It includes Go, Node, Python, and .NET pre-installed so tool-install steps
// are no-ops rather than full downloads.
const DefaultImage = "ghcr.io/kascea/stepthrough-agent:latest"

// IsDockerAvailable returns true if the Docker daemon is reachable.
func IsDockerAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}

// ImageExistsLocally returns true if the image is already present in the local Docker cache.
func ImageExistsLocally(image string) bool {
	return exec.Command("docker", "image", "inspect", "--format", "{{.Id}}", image).Run() == nil
}

// ResolveImage maps an Azure vmImage to a local Docker image.
// ubuntu-latest and ubuntu-22.04 resolve to the stepthrough-agent image which
// mirrors the Azure hosted-agent tool set. Returns ("", false) for unsupported platforms.
func ResolveImage(vmImage string) (image string, ok bool) {
	switch vmImage {
	case "", "ubuntu-latest", "ubuntu-22.04":
		return DefaultImage, true
	case "windows-latest", "windows-2022", "windows-2019",
		"macos-latest", "macos-13", "macos-12":
		return "", false
	default:
		// Treat as a custom Docker image reference and pass through.
		return vmImage, true
	}
}
