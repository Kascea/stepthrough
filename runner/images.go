package runner

// DefaultImage is the only supported Docker image for local debugging.
const DefaultImage = "ubuntu:22.04"

// ResolveImage maps an Azure vmImage to a local Docker image.
// Currently only ubuntu-latest is supported (running on macOS via Docker Desktop).
// Returns ("", false) for unsupported platforms.
func ResolveImage(vmImage string) (image string, ok bool) {
	switch vmImage {
	case "", "ubuntu-latest", "ubuntu-22.04":
		return DefaultImage, true
	default:
		return "", false
	}
}
