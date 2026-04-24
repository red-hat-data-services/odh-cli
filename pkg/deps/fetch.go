package deps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	gitopsBaseURL = "https://raw.githubusercontent.com/opendatahub-io/odh-gitops"
	valuesPath    = "charts/rhai-on-openshift-chart/values.yaml"
	chartPath     = "charts/rhai-on-openshift-chart/Chart.yaml"
	fetchTimeout  = 30 * time.Second
	maxFetchSize  = 5 * 1024 * 1024 // 5MB max response size

	msgFetchManifest = "fetch latest manifest: %w"
	msgFetchChart    = "fetch chart metadata: %w"
	msgCreateRequest = "create request: %w"
	msgFetchFile     = "fetch file: %w"
	msgFetchHTTP     = "fetch file %s: HTTP %d"
	msgReadResponse  = "read response: %w"
)

// gitopsRef is the default commit SHA for fetching manifests.
// Can be overridden at build time via ldflags or at runtime via ODH_GITOPS_COMMIT env var.
//
//nolint:gochecknoglobals // build-time injection requires package-level var
var gitopsRef = "1a55af06b8fe85c8ed63b1eff680477d9bf86be3"

// getGitopsRef returns the gitops commit ref to use.
// Priority: ODH_GITOPS_COMMIT env var > build-time ldflags > default.
func getGitopsRef() string {
	if ref := os.Getenv("ODH_GITOPS_COMMIT"); ref != "" {
		return ref
	}

	return gitopsRef
}

// ErrEmbeddedEmpty is returned when the embedded manifest is empty.
var ErrEmbeddedEmpty = errors.New("embedded manifest is empty")

// ManifestResult contains the parsed manifest and its version.
type ManifestResult struct {
	Manifest *Manifest
	Version  string
}

// GetManifest returns the parsed manifest and version, using embedded data by default.
// If refresh is true, fetches the latest manifest from GitHub.
//
//nolint:revive // refresh flag is intentional API design for CLI command
func GetManifest(ctx context.Context, refresh bool) (*ManifestResult, error) {
	if refresh {
		return fetchAndParseManifest(ctx, gitopsBaseURL)
	}

	return parseEmbeddedManifest()
}

func fetchAndParseManifest(ctx context.Context, baseURL string) (*ManifestResult, error) {
	valuesData, err := fetchFile(ctx, baseURL, valuesPath)
	if err != nil {
		return nil, fmt.Errorf(msgFetchManifest, err)
	}

	manifest, err := Parse(valuesData)
	if err != nil {
		return nil, err
	}

	chartData, err := fetchFile(ctx, baseURL, chartPath)
	if err != nil {
		return nil, fmt.Errorf(msgFetchChart, err)
	}

	version, err := parseChartVersion(chartData)
	if err != nil {
		return nil, fmt.Errorf(msgFetchChart, err)
	}

	return &ManifestResult{
		Manifest: manifest,
		Version:  version,
	}, nil
}

func parseEmbeddedManifest() (*ManifestResult, error) {
	data := EmbeddedManifest()
	if len(data) == 0 {
		return nil, ErrEmbeddedEmpty
	}

	manifest, err := Parse(data)
	if err != nil {
		return nil, err
	}

	version, err := ManifestVersion()
	if err != nil {
		return nil, err
	}

	return &ManifestResult{
		Manifest: manifest,
		Version:  version,
	}, nil
}

// fetchFile downloads a file from a given base URL and path.
func fetchFile(ctx context.Context, baseURL string, path string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s/%s", baseURL, getGitopsRef(), path)

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf(msgCreateRequest, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf(msgFetchFile, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(msgFetchHTTP, path, resp.StatusCode)
	}

	// Read up to maxFetchSize + 1 to detect overflow
	limitedReader := io.LimitReader(resp.Body, maxFetchSize+1)

	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf(msgReadResponse, err)
	}

	if len(data) > maxFetchSize {
		return nil, fmt.Errorf("response exceeds maximum size of %d bytes", maxFetchSize)
	}

	return data, nil
}

// parseChartVersion extracts appVersion from Chart.yaml content.
func parseChartVersion(data []byte) (string, error) {
	var chart chartMetadata
	if err := yaml.Unmarshal(data, &chart); err != nil {
		return "", fmt.Errorf("parse Chart.yaml: %w", err)
	}

	if chart.AppVersion == "" {
		return "", errors.New("parse Chart.yaml: appVersion field is empty")
	}

	return chart.AppVersion, nil
}
