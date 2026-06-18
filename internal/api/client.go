// Package api provides the HTTP client for communicating with the
// RouteStack control plane over mTLS.
package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

// Client is the mTLS HTTP client for controller communication.
// All methods use context-aware retry with exponential backoff.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// RetryConfig controls the retry behavior.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
}

// DefaultRetryConfig is used when no explicit config is provided.
var DefaultRetryConfig = RetryConfig{
	MaxRetries: 5,
	BaseDelay:  1 * time.Second,
}

// NewClient creates an mTLS HTTP client that authenticates with the
// provided client certificate and verifies the server against the CA.
func NewClient(baseURL, certPath, keyPath, caCertPath string, connectTimeout, requestTimeout time.Duration) (*Client, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load client cert: %w", err)
	}

	caPool, err := loadCAPool(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("load CA cert: %w", err)
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}

	return newClientWithTLS(baseURL, tlsCfg, connectTimeout, requestTimeout), nil
}

// NewClientWithCA creates a TLS client that only verifies the server
// against the CA certificate — no client certificate is presented.
// This is used for the initial enrollment request when the agent
// does not yet have its own certificate.
func NewClientWithCA(baseURL, caCertPath string, connectTimeout, requestTimeout time.Duration) (*Client, error) {
	caPool, err := loadCAPool(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("load CA cert: %w", err)
	}

	tlsCfg := &tls.Config{
		RootCAs:    caPool,
		MinVersion: tls.VersionTLS12,
	}

	return newClientWithTLS(baseURL, tlsCfg, connectTimeout, requestTimeout), nil
}

func newClientWithTLS(baseURL string, tlsCfg *tls.Config, connectTimeout, requestTimeout time.Duration) *Client {
	transport := &http.Transport{
		TLSClientConfig: tlsCfg,
		DialContext: (&net.Dialer{
			Timeout: connectTimeout,
		}).DialContext,
		MaxIdleConns:    10,
		IdleConnTimeout: 90 * time.Second,
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   requestTimeout,
		},
		baseURL: baseURL,
	}
}

func loadCAPool(caCertPath string) (*x509.CertPool, error) {
	pemData, err := os.ReadFile(caCertPath)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("no valid CA certificates found in %s", caCertPath)
	}
	return pool, nil
}

// Enroll sends an enrollment request to the controller.
func (c *Client) Enroll(ctx context.Context, req EnrollRequest) (*EnrollResponse, error) {
	var resp EnrollResponse
	err := c.doWithRetry(ctx, http.MethodPost, "/internal/agent/v1/enroll", req, &resp, DefaultRetryConfig)
	return &resp, err
}

// Heartbeat sends a heartbeat payload to the controller.
func (c *Client) Heartbeat(ctx context.Context, req HeartbeatRequest) (*HeartbeatResponse, error) {
	var resp HeartbeatResponse
	err := c.doWithRetry(ctx, http.MethodPost, "/internal/agent/v1/heartbeat", req, &resp, DefaultRetryConfig)
	return &resp, err
}

// HeartbeatResponse is the controller's reply to a heartbeat.
type HeartbeatResponse struct {
	Accepted bool `json:"accepted"`
}

// ClaimOperation long-polls for a pending operation assigned to this node.
func (c *Client) ClaimOperation(ctx context.Context, req ClaimRequest) (*ClaimResponse, error) {
	var resp ClaimResponse
	err := c.doWithRetry(ctx, http.MethodPost, "/internal/agent/v1/operations/claim", req, &resp, DefaultRetryConfig)
	return &resp, err
}

// ClaimRequest is the body sent when long-polling for work.
type ClaimRequest struct {
	NodeID string `json:"node_id"`
}

// ClaimResponse contains an operation if one is available.
type ClaimResponse struct {
	Operation *Operation `json:"operation,omitempty"`
}

// Operation is a single task assigned to the agent by the controller.
type Operation struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// CompleteRequest is the body sent when an operation finishes.
type CompleteRequest struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Result string `json:"result,omitempty"`
}

// CompleteOperation reports the result of an operation back to the controller.
func (c *Client) CompleteOperation(ctx context.Context, opID string, req CompleteRequest) error {
	path := fmt.Sprintf("/internal/agent/v1/operations/%s/complete", opID)
	return c.doWithRetry(ctx, http.MethodPost, path, req, nil, DefaultRetryConfig)
}

// doWithRetry performs an HTTP request with exponential-backoff retries.
// It retries on 5xx, connection errors, and timeouts.
// It does NOT retry on 4xx responses (except 408 Request Timeout and 429 Too Many Requests).
func (c *Client) doWithRetry(ctx context.Context, method, path string, body, result any, cfg RetryConfig) error {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := cfg.BaseDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		// Re-create the body reader for retries since it may have been consumed.
		var bodyReader io.Reader
		if body != nil {
			data, _ := json.Marshal(body)
			bodyReader = bytes.NewReader(data)
		} else {
			bodyReader = reqBody
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if isRetryableError(err) && attempt < cfg.MaxRetries {
				continue
			}
			return fmt.Errorf("request %s %s: %w", method, path, err)
		}

		respData, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		if readErr != nil && attempt < cfg.MaxRetries {
			continue
		}
		if readErr != nil {
			return fmt.Errorf("read response body: %w", readErr)
		}

		if isRetryableStatus(resp.StatusCode) && attempt < cfg.MaxRetries {
			continue
		}

		if resp.StatusCode >= 400 {
			return &HTTPError{
				StatusCode: resp.StatusCode,
				Body:       string(respData),
			}
		}

		if result != nil {
			if err := json.Unmarshal(respData, result); err != nil {
				return fmt.Errorf("unmarshal response: %w", err)
			}
		}
		return nil
	}

	return fmt.Errorf("request %s %s: exhausted %d retries", method, path, cfg.MaxRetries)
}

// isRetryableStatus returns true for 5xx, 408, and 429 status codes.
func isRetryableStatus(code int) bool {
	if code >= 500 {
		return true
	}
	return code == 408 || code == 429
}

// isRetryableError returns true for connection-level errors that are worth retrying.
func isRetryableError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// Retry on DNS, connection refused, reset, timeout.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return true
}

// HTTPError is a non-retryable HTTP error response.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http %d: %s", e.StatusCode, e.Body)
}
