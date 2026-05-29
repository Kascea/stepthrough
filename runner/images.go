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
	case "windows-latest", "windows-2022", "windows-2019",
		"macos-latest", "macos-13", "macos-12":
		return "", false
	default:
		// Treat as a custom Docker image reference and pass through.
		return vmImage, true
	}
}
