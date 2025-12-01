package api

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	registryBaseURL = "https://registry-1.docker.io/v2"
	authURL         = "https://auth.docker.io/token"
	registryService = "registry.docker.io"
	manifestAccept  = "application/vnd.docker.distribution.manifest.v2+json"
	manifestListAccept = "application/vnd.docker.distribution.manifest.list.v2+json"
)

// RegistryClient handles Docker Registry API v2 requests
type RegistryClient struct {
	httpClient *http.Client
	token      string // cached bearer token
	user       string // cached user from last request
	repo       string // cached repo from last request
}

// Manifest represents a Docker image manifest
type Manifest struct {
	MediaType string
	Config    BlobRef
	Layers    []Layer
	Platforms []Platform // populated if manifest list
	V1History []V1HistoryEntry // populated for v1 manifests
}

// V1HistoryEntry represents a history entry in v1 manifests
type V1HistoryEntry struct {
	V1Compatibility string `json:"v1Compatibility"`
}

// BlobRef references a blob (config or layer)
type BlobRef struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
}

// Platform represents an OS/architecture combination
type Platform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Variant      string `json:"variant,omitempty"`
	Digest       string // manifest digest for this platform
}

// Layer represents a single image layer
type Layer struct {
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// TarEntry represents a file or directory in a tar archive
type TarEntry struct {
	Name  string
	Size  int64
	IsDir bool
}

// NewRegistryClient creates a new Docker Registry API client
func NewRegistryClient() *RegistryClient {
	return &RegistryClient{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// ParseImageRef parses an image reference into user, repo, and tag
// Examples:
//   - "nginx" -> "library", "nginx", "latest"
//   - "nginx:1.21" -> "library", "nginx", "1.21"
//   - "moby/buildkit" -> "moby", "buildkit", "latest"
//   - "moby/buildkit:v0.12" -> "moby", "buildkit", "v0.12"
func ParseImageRef(ref string) (user, repo, tag string) {
	tag = "latest"

	// Split off tag if present
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		// Make sure it's not a port number (no slashes after colon)
		afterColon := ref[idx+1:]
		if !strings.Contains(afterColon, "/") {
			tag = afterColon
			ref = ref[:idx]
		}
	}

	// Split user/repo
	if strings.Contains(ref, "/") {
		parts := strings.SplitN(ref, "/", 2)
		user = parts[0]
		repo = parts[1]
	} else {
		user = "library"
		repo = ref
	}

	return user, repo, tag
}

// FetchPullToken retrieves an anonymous bearer token for pulling from Docker Hub
func (c *RegistryClient) FetchPullToken(user, repo string) (string, error) {
	url := fmt.Sprintf("%s?service=%s&scope=repository:%s/%s:pull", authURL, registryService, user, repo)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned status %d", resp.StatusCode)
	}

	var tokenResp struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResp.Token == "" {
		return "", fmt.Errorf("token endpoint returned empty token")
	}

	c.token = tokenResp.Token
	c.user = user
	c.repo = repo

	return tokenResp.Token, nil
}

// ListTags fetches available tags for an image repository
func (c *RegistryClient) ListTags(imageRef string) ([]string, error) {
	user, repo, _ := ParseImageRef(imageRef)

	url := fmt.Sprintf("%s/tags/list", registryURL(user, repo))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tags: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tags request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tagsResp struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, fmt.Errorf("failed to decode tags response: %w", err)
	}

	return tagsResp.Tags, nil
}

// registryURL builds a registry URL for the given user/repo
func registryURL(user, repo string) string {
	return fmt.Sprintf("%s/%s/%s", registryBaseURL, user, repo)
}

// doRequest performs an HTTP request with auth handling
func (c *RegistryClient) doRequest(req *http.Request) (*http.Response, error) {
	// Set accept header for manifests - request v2 manifest (like Python version)
	// The registry will return a manifest list if the image is multi-arch
	if strings.Contains(req.URL.Path, "/manifests/") {
		req.Header.Set("Accept", manifestAccept)
	}

	// Add auth if we have a token
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Handle 401 by fetching a new token
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()

		// Extract user/repo from URL
		parts := strings.Split(req.URL.Path, "/")
		if len(parts) >= 4 {
			user := parts[2]
			repo := parts[3]
			if _, err := c.FetchPullToken(user, repo); err != nil {
				return nil, fmt.Errorf("failed to refresh token: %w", err)
			}

			// Retry with new token
			req.Header.Set("Authorization", "Bearer "+c.token)
			return c.httpClient.Do(req)
		}
	}

	return resp, nil
}

// GetManifest fetches the manifest for an image
// If digest is empty, fetches by tag. Otherwise fetches by digest.
func (c *RegistryClient) GetManifest(imageRef string, digest string) (*Manifest, error) {
	user, repo, tag := ParseImageRef(imageRef)

	ref := tag
	if digest != "" {
		ref = digest
	}

	url := fmt.Sprintf("%s/manifests/%s", registryURL(user, repo), ref)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("manifest request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest body: %w", err)
	}

	// Check content type to determine if it's a manifest list or single manifest
	contentType := resp.Header.Get("Content-Type")

	manifest := &Manifest{
		MediaType: contentType,
	}

	// Check if this is a manifest list by looking for "manifests" key in JSON
	// (like Python version does with manifest_index.get("manifests"))
	var rawManifest map[string]interface{}
	if err := json.Unmarshal(body, &rawManifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest JSON: %w", err)
	}

	if _, hasManifests := rawManifest["manifests"]; hasManifests {
		// Parse as manifest list (multi-arch)
		var listResp struct {
			Manifests []struct {
				MediaType string `json:"mediaType"`
				Digest    string `json:"digest"`
				Size      int64  `json:"size"`
				Platform  struct {
					Architecture string `json:"architecture"`
					OS           string `json:"os"`
					Variant      string `json:"variant,omitempty"`
				} `json:"platform"`
			} `json:"manifests"`
		}
		if err := json.Unmarshal(body, &listResp); err != nil {
			return nil, fmt.Errorf("failed to parse manifest list: %w", err)
		}

		for _, m := range listResp.Manifests {
			manifest.Platforms = append(manifest.Platforms, Platform{
				OS:           m.Platform.OS,
				Architecture: m.Platform.Architecture,
				Variant:      m.Platform.Variant,
				Digest:       m.Digest,
			})
		}
	} else if _, hasFsLayers := rawManifest["fsLayers"]; hasFsLayers {
		// Parse as v1 manifest (schemaVersion 1)
		// v1 manifests use "fsLayers" with "blobSum" instead of "layers" with "digest"
		var v1Resp struct {
			SchemaVersion int    `json:"schemaVersion"`
			Name          string `json:"name"`
			Tag           string `json:"tag"`
			Architecture  string `json:"architecture"`
			FsLayers      []struct {
				BlobSum string `json:"blobSum"`
			} `json:"fsLayers"`
			History []V1HistoryEntry `json:"history"`
		}
		if err := json.Unmarshal(body, &v1Resp); err != nil {
			return nil, fmt.Errorf("failed to parse v1 manifest: %w", err)
		}

		// v1 manifests list layers in reverse order (top-most layer first in fsLayers)
		// We need to reverse them to get bottom-to-top order like v2
		// Also deduplicate (v1 manifests often have duplicate empty layers)
		seen := make(map[string]bool)
		for i := len(v1Resp.FsLayers) - 1; i >= 0; i-- {
			digest := v1Resp.FsLayers[i].BlobSum
			if !seen[digest] {
				seen[digest] = true
				manifest.Layers = append(manifest.Layers, Layer{
					Digest: digest,
					Size:   0, // v1 manifests don't include size in fsLayers
				})
			}
		}

		// For v1 manifests, extract build history from v1Compatibility JSON strings
		// Store the first history entry's config info for build steps extraction
		manifest.MediaType = "application/vnd.docker.distribution.manifest.v1+json"
		manifest.V1History = v1Resp.History

	} else {
		// Parse as v2 single manifest
		var singleResp struct {
			Config BlobRef   `json:"config"`
			Layers []BlobRef `json:"layers"`
		}
		if err := json.Unmarshal(body, &singleResp); err != nil {
			return nil, fmt.Errorf("failed to parse manifest: %w", err)
		}

		manifest.Config = singleResp.Config
		for _, l := range singleResp.Layers {
			manifest.Layers = append(manifest.Layers, Layer{
				Digest: l.Digest,
				Size:   l.Size,
			})
		}
	}

	return manifest, nil
}

// FetchBuildSteps retrieves the Dockerfile history from the image config
func (c *RegistryClient) FetchBuildSteps(imageRef, configDigest string) ([]string, error) {
	user, repo, _ := ParseImageRef(imageRef)

	url := fmt.Sprintf("%s/blobs/%s", registryURL(user, repo), configDigest)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("config request failed with status %d", resp.StatusCode)
	}

	var config struct {
		History []struct {
			CreatedBy  string `json:"created_by"`
			EmptyLayer bool   `json:"empty_layer,omitempty"`
		} `json:"history"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	var steps []string
	for _, entry := range config.History {
		step := strings.TrimSpace(entry.CreatedBy)
		if entry.EmptyLayer {
			step += " (metadata only)"
		}
		steps = append(steps, step)
	}

	return steps, nil
}

// ExtractV1BuildSteps extracts build steps from v1 manifest history
// v1 manifests embed the config in v1Compatibility JSON strings
func ExtractV1BuildSteps(history []V1HistoryEntry) []string {
	var steps []string

	// v1 history is in reverse order (newest first), so we reverse it
	for i := len(history) - 1; i >= 0; i-- {
		entry := history[i]

		// Parse the v1Compatibility JSON
		var compat struct {
			ContainerConfig struct {
				Cmd []string `json:"Cmd"`
			} `json:"container_config"`
			Created string `json:"created"`
		}

		if err := json.Unmarshal([]byte(entry.V1Compatibility), &compat); err != nil {
			continue
		}

		// Extract the command - usually the last element is the actual command
		if len(compat.ContainerConfig.Cmd) > 0 {
			cmd := compat.ContainerConfig.Cmd[len(compat.ContainerConfig.Cmd)-1]
			// Clean up the command string
			cmd = strings.TrimPrefix(cmd, "/bin/sh -c ")
			cmd = strings.TrimPrefix(cmd, "#(nop) ")
			cmd = strings.TrimSpace(cmd)
			if cmd != "" {
				steps = append(steps, cmd)
			}
		}
	}

	return steps
}

// PeekLayerBlob downloads a layer and lists its contents without saving to disk
func (c *RegistryClient) PeekLayerBlob(imageRef, digest string) ([]TarEntry, error) {
	user, repo, _ := ParseImageRef(imageRef)

	url := fmt.Sprintf("%s/blobs/%s", registryURL(user, repo), digest)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch layer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("layer request failed with status %d", resp.StatusCode)
	}

	// Decompress gzip
	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Read tar entries
	tarReader := tar.NewReader(gzReader)
	var entries []TarEntry

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar entry: %w", err)
		}

		entries = append(entries, TarEntry{
			Name:  header.Name,
			Size:  header.Size,
			IsDir: header.Typeflag == tar.TypeDir,
		})
	}

	return entries, nil
}

// DownloadLayerBlob downloads a layer blob to disk
func (c *RegistryClient) DownloadLayerBlob(imageRef, digest string, size int64) (string, error) {
	user, repo, _ := ParseImageRef(imageRef)

	url := fmt.Sprintf("%s/blobs/%s", registryURL(user, repo), digest)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.doRequest(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch layer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("layer request failed with status %d", resp.StatusCode)
	}

	// Create output directory
	outputDir := filepath.Join("downloads", fmt.Sprintf("%s_%s", user, repo), "latest")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create output file
	filename := strings.ReplaceAll(digest, ":", "_") + ".tar.gz"
	outputPath := filepath.Join(outputDir, filename)

	file, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Stream to file
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write layer to file: %w", err)
	}

	return outputPath, nil
}

// HumanReadableSize converts bytes to a human-readable string
func HumanReadableSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

