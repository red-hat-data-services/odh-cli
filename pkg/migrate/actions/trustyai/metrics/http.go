package metrics

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

const (
	defaultRouteLabel  = "trustyai-service-name=trustyai-service"
	fallbackRouteLabel = "app=trustyai-service"
	defaultRouteName   = "trustyai-service"
	maxResponseSize    = 10 << 20 // 10 MiB
)

//nolint:gochecknoglobals // Static endpoint mapping; Go has no const maps.
var metricEndpoints = map[string]string{
	"spd":          "/metrics/group/fairness/spd/request",
	"dir":          "/metrics/group/fairness/dir/request",
	"meanshift":    "/metrics/drift/meanshift/request",
	"kstest":       "/metrics/drift/kstest/request",
	"approxkstest": "/metrics/drift/approxkstest/request",
	"fouriermmd":   "/metrics/drift/fouriermmd/request",
}

type httpHelper struct {
	client      *http.Client
	bearerToken string
}

func newHTTPHelper(restConfig *rest.Config) (*httpHelper, error) {
	token := ""

	if restConfig != nil {
		token = restConfig.BearerToken
		if token == "" && restConfig.BearerTokenFile != "" {
			tokenBytes, err := os.ReadFile(restConfig.BearerTokenFile)
			if err != nil {
				return nil, fmt.Errorf("reading bearer token file: %w", err)
			}

			token = strings.TrimSpace(string(tokenBytes))
		}
	}

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				//nolint:gosec // In-cluster routes use self-signed certs; matches original curl -sk behavior.
				InsecureSkipVerify: true,
			},
		},
	}

	return &httpHelper{client: httpClient, bearerToken: token}, nil
}

func (h *httpHelper) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating GET request: %w", err)
	}

	if h.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.bearerToken)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing GET request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (h *httpHelper) post(ctx context.Context, url string, payload []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("creating POST request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if h.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+h.bearerToken)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("executing POST request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
}

func discoverRouteInNamespace(ctx context.Context, k8sClient client.Client, namespace, routeLabel string) (string, error) {
	host, err := findRouteByLabel(ctx, k8sClient, namespace, routeLabel)
	if err != nil {
		return "", err
	}

	if host != "" {
		return host, nil
	}

	if routeLabel != fallbackRouteLabel {
		host, err = findRouteByLabel(ctx, k8sClient, namespace, fallbackRouteLabel)
		if err != nil {
			return "", err
		}

		if host != "" {
			return host, nil
		}
	}

	host, err = findRouteByName(ctx, k8sClient, namespace, defaultRouteName)
	if err != nil {
		return "", err
	}

	return host, nil
}

func findRouteByLabel(ctx context.Context, k8sClient client.Client, namespace, label string) (string, error) {
	routes, err := k8sClient.List(ctx, resources.Route,
		client.WithNamespace(namespace),
		client.WithLabelSelector(label),
		client.WithLimit(1),
	)
	if err != nil {
		return "", fmt.Errorf("listing routes with label %q: %w", label, err)
	}

	if len(routes) == 0 {
		return "", nil
	}

	return extractRouteHost(routes[0])
}

func findRouteByName(ctx context.Context, k8sClient client.Client, namespace, name string) (string, error) {
	route, err := k8sClient.GetResource(ctx, resources.Route, name, client.InNamespace(namespace))
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return "", nil
		}

		return "", fmt.Errorf("getting route %q: %w", name, err)
	}

	if route == nil {
		return "", nil
	}

	return extractRouteHost(route)
}

func extractRouteHost(route *unstructured.Unstructured) (string, error) {
	host, found, err := unstructured.NestedString(route.Object, "spec", "host")
	if err != nil || !found || host == "" {
		return "", nil
	}

	return host, nil
}

type backupResponse struct {
	Requests []json.RawMessage `json:"requests"`
}

type restoreEntry struct {
	ID      string          `json:"id"`
	Request restoreRequest  `json:"request"`
	Raw     json.RawMessage `json:"-"`
}

type restoreRequest struct {
	MetricName string `json:"metricName"`
	ModelID    string `json:"modelId"`
}
