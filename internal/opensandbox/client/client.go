package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

const (
	execdPort = 44772
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	maxRetries int
	retryWait  time.Duration
}

type Config struct {
	BaseURL    string
	APIKey     string
	Timeout    time.Duration
	MaxRetries int
	RetryWait  time.Duration
}

type ProviderError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	StatusCode int    `json:"status_code"`
	Retryable  bool   `json:"retryable"`
}

func (e ProviderError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type OSImageAuth struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type OSImageSpec struct {
	URI  string       `json:"uri"`
	Auth *OSImageAuth `json:"auth,omitempty"`
}

type OSResourceLimits map[string]string

type OSEgressRule struct {
	Action string `json:"action"`
	Target string `json:"target"`
}

type OSNetworkPolicy struct {
	DefaultAction string         `json:"defaultAction,omitempty"`
	Egress        []OSEgressRule `json:"egress,omitempty"`
}

type OSSandboxCreateRequest struct {
	Image          OSImageSpec        `json:"image"`
	Timeout        int                `json:"timeout"`
	ResourceLimits OSResourceLimits   `json:"resourceLimits"`
	Env            map[string]*string `json:"env,omitempty"`
	Metadata       map[string]string  `json:"metadata,omitempty"`
	Entrypoint     []string           `json:"entrypoint"`
	NetworkPolicy  *OSNetworkPolicy   `json:"networkPolicy,omitempty"`
	Extensions     map[string]string  `json:"extensions,omitempty"`
}

type OSSandboxStatus struct {
	State            string     `json:"state"`
	Reason           string     `json:"reason,omitempty"`
	Message          string     `json:"message,omitempty"`
	LastTransitionAt *time.Time `json:"lastTransitionAt,omitempty"`
}

type OSSandboxInfo struct {
	ID         string            `json:"id"`
	Status     OSSandboxStatus   `json:"status"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	EntryPoint []string          `json:"entrypoint,omitempty"`
	ExpiresAt  *time.Time        `json:"expiresAt,omitempty"`
	CreatedAt  *time.Time        `json:"createdAt,omitempty"`
}

type OSEndpoint struct {
	Endpoint string            `json:"endpoint"`
	Headers  map[string]string `json:"headers,omitempty"`
}

type OSRenewSandboxExpirationRequest struct {
	ExpiresAt time.Time `json:"expiresAt"`
}

type OSRenewSandboxExpirationResponse struct {
	ExpiresAt time.Time `json:"expiresAt"`
}

type OSExecRequest struct {
	Command    string            `json:"command"`
	Cwd        string            `json:"cwd,omitempty"`
	Background bool              `json:"background,omitempty"`
	TimeoutMs  int64             `json:"timeout,omitempty"`
	Envs       map[string]string `json:"envs,omitempty"`
}

type OSServerStreamEvent struct {
	Type           string         `json:"type,omitempty"`
	Text           string         `json:"text,omitempty"`
	ExecutionCount int            `json:"execution_count,omitempty"`
	ExecutionTime  int64          `json:"execution_time,omitempty"`
	Timestamp      int64          `json:"timestamp,omitempty"`
	Results        map[string]any `json:"results,omitempty"`
	Error          *OSExecError   `json:"error,omitempty"`
}

type OSExecError struct {
	EName     string   `json:"ename,omitempty"`
	EValue    string   `json:"evalue,omitempty"`
	Traceback []string `json:"traceback,omitempty"`
}

type OSCommandStatus struct {
	ID         string     `json:"id"`
	Content    string     `json:"content,omitempty"`
	Running    bool       `json:"running"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type OSFileMetadata struct {
	Path  string `json:"path"`
	Owner string `json:"owner,omitempty"`
	Group string `json:"group,omitempty"`
	Mode  int    `json:"mode,omitempty"`
}

func New(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	retryWait := cfg.RetryWait
	if retryWait <= 0 {
		retryWait = 500 * time.Millisecond
	}
	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		maxRetries: cfg.MaxRetries,
		retryWait:  retryWait,
	}
}

func (c *Client) CreateSandbox(ctx context.Context, req OSSandboxCreateRequest) (OSSandboxInfo, error) {
	var out OSSandboxInfo
	err := c.doJSON(ctx, http.MethodPost, "/v1/sandboxes", req, &out, nil)
	return out, err
}

func (c *Client) GetSandbox(ctx context.Context, sandboxID string) (OSSandboxInfo, error) {
	var out OSSandboxInfo
	err := c.doJSON(ctx, http.MethodGet, "/v1/sandboxes/"+url.PathEscape(sandboxID), nil, &out, nil)
	return out, err
}

func (c *Client) DeleteSandbox(ctx context.Context, sandboxID string) error {
	return c.doJSON(ctx, http.MethodDelete, "/v1/sandboxes/"+url.PathEscape(sandboxID), nil, nil, nil)
}

func (c *Client) PauseSandbox(ctx context.Context, sandboxID string) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/sandboxes/"+url.PathEscape(sandboxID)+"/pause", nil, nil, nil)
}

func (c *Client) ResumeSandbox(ctx context.Context, sandboxID string) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/sandboxes/"+url.PathEscape(sandboxID)+"/resume", nil, nil, nil)
}

func (c *Client) RenewSandbox(ctx context.Context, sandboxID string, expiresAt time.Time) (OSRenewSandboxExpirationResponse, error) {
	var out OSRenewSandboxExpirationResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/sandboxes/"+url.PathEscape(sandboxID)+"/renew-expiration", OSRenewSandboxExpirationRequest{ExpiresAt: expiresAt}, &out, nil)
	return out, err
}

func (c *Client) GetSandboxEndpoint(ctx context.Context, sandboxID string, port int, useServerProxy bool) (OSEndpoint, error) {
	query := url.Values{}
	if useServerProxy {
		query.Set("use_server_proxy", "true")
	}
	p := fmt.Sprintf("/v1/sandboxes/%s/endpoints/%d", url.PathEscape(sandboxID), port)
	if encoded := query.Encode(); encoded != "" {
		p += "?" + encoded
	}
	var out OSEndpoint
	err := c.doJSON(ctx, http.MethodGet, p, nil, &out, nil)
	if err != nil {
		return OSEndpoint{}, err
	}
	out.Endpoint = normalizeEndpoint(c.baseURL, out.Endpoint)
	return out, nil
}

func (c *Client) RunCommandStream(ctx context.Context, sandboxID string, req OSExecRequest) (io.ReadCloser, error) {
	endpoint, err := c.GetSandboxEndpoint(ctx, sandboxID, execdPort, true)
	if err != nil {
		return nil, err
	}
	headers := copyHeaders(endpoint.Headers)
	headers["Content-Type"] = "application/json"
	headers["Accept"] = "text/event-stream"
	resp, err := c.doRaw(ctx, http.MethodPost, endpoint.Endpoint+"/command", req, headers)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *Client) GetCommandStatus(ctx context.Context, sandboxID, commandID string) (OSCommandStatus, error) {
	endpoint, err := c.GetSandboxEndpoint(ctx, sandboxID, execdPort, true)
	if err != nil {
		return OSCommandStatus{}, err
	}
	var out OSCommandStatus
	err = c.doJSONAbsolute(ctx, http.MethodGet, endpoint.Endpoint+"/command/status/"+url.PathEscape(commandID), nil, &out, endpoint.Headers)
	return out, err
}

func (c *Client) GetBackgroundCommandLogs(ctx context.Context, sandboxID, commandID string, cursor int64) (string, int64, error) {
	endpoint, err := c.GetSandboxEndpoint(ctx, sandboxID, execdPort, true)
	if err != nil {
		return "", cursor, err
	}
	reqURL := endpoint.Endpoint + "/command/" + url.PathEscape(commandID) + "/logs?cursor=" + url.QueryEscape(fmt.Sprintf("%d", cursor))
	resp, err := c.doRaw(ctx, http.MethodGet, reqURL, nil, endpoint.Headers)
	if err != nil {
		return "", cursor, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", cursor, err
	}
	nextCursor := cursor
	if value := resp.Header.Get("EXECD-COMMANDS-TAIL-CURSOR"); value != "" {
		fmt.Sscanf(value, "%d", &nextCursor)
	}
	return string(data), nextCursor, nil
}

func (c *Client) InterruptCommand(ctx context.Context, sandboxID, commandID string) error {
	endpoint, err := c.GetSandboxEndpoint(ctx, sandboxID, execdPort, true)
	if err != nil {
		return err
	}
	reqURL := endpoint.Endpoint + "/command?id=" + url.QueryEscape(commandID)
	return c.doJSONAbsolute(ctx, http.MethodDelete, reqURL, nil, nil, endpoint.Headers)
}

func (c *Client) UploadFile(ctx context.Context, sandboxID, localPath, remotePath string) error {
	endpoint, err := c.GetSandboxEndpoint(ctx, sandboxID, execdPort, true)
	if err != nil {
		return err
	}

	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	meta, err := writer.CreateFormFile("metadata", "metadata.json")
	if err != nil {
		return err
	}
	metaPayload, err := json.Marshal(OSFileMetadata{Path: remotePath})
	if err != nil {
		return err
	}
	if _, err := meta.Write(metaPayload); err != nil {
		return err
	}
	part, err := writer.CreateFormFile("file", path.Base(remotePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	headers := copyHeaders(endpoint.Headers)
	headers["Content-Type"] = writer.FormDataContentType()
	return c.doJSONAbsolute(ctx, http.MethodPost, endpoint.Endpoint+"/files/upload", &body, nil, headers)
}

func (c *Client) DownloadFile(ctx context.Context, sandboxID, remotePath, localPath string) error {
	endpoint, err := c.GetSandboxEndpoint(ctx, sandboxID, execdPort, true)
	if err != nil {
		return err
	}
	reqURL := endpoint.Endpoint + "/files/download?path=" + url.QueryEscape(remotePath)
	resp, err := c.doRaw(ctx, http.MethodGet, reqURL, nil, endpoint.Headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := os.MkdirAll(pathDir(localPath), 0o755); err != nil {
		return err
	}
	file, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(file, resp.Body)
	return err
}

func (c *Client) MakeDirs(ctx context.Context, sandboxID string, dirs map[string]int) error {
	endpoint, err := c.GetSandboxEndpoint(ctx, sandboxID, execdPort, true)
	if err != nil {
		return err
	}
	payload := map[string]map[string]int{}
	for dir, mode := range dirs {
		payload[dir] = map[string]int{"mode": mode}
	}
	return c.doJSONAbsolute(ctx, http.MethodPost, endpoint.Endpoint+"/directories", payload, nil, endpoint.Headers)
}

func (c *Client) RemoveDirs(ctx context.Context, sandboxID string, dirs []string) error {
	endpoint, err := c.GetSandboxEndpoint(ctx, sandboxID, execdPort, true)
	if err != nil {
		return err
	}
	query := url.Values{}
	for _, dir := range dirs {
		query.Add("path", dir)
	}
	reqURL := endpoint.Endpoint + "/directories"
	if encoded := query.Encode(); encoded != "" {
		reqURL += "?" + encoded
	}
	return c.doJSONAbsolute(ctx, http.MethodDelete, reqURL, nil, nil, endpoint.Headers)
}

func (c *Client) doJSON(ctx context.Context, method, reqPath string, reqBody any, respBody any, headers map[string]string) error {
	return c.doJSONAbsolute(ctx, method, c.baseURL+reqPath, reqBody, respBody, headers)
}

func (c *Client) doJSONAbsolute(ctx context.Context, method, rawURL string, reqBody any, respBody any, headers map[string]string) error {
	resp, err := c.doRaw(ctx, method, rawURL, reqBody, headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if respBody == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(respBody)
}

func (c *Client) doRaw(ctx context.Context, method, rawURL string, reqBody any, headers map[string]string) (*http.Response, error) {
	var body io.Reader
	switch value := reqBody.(type) {
	case nil:
	case io.Reader:
		body = value
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	if reqBody != nil {
		if _, ok := reqBody.(io.Reader); !ok {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("OPEN-SANDBOX-API-KEY", c.apiKey)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	var resp *http.Response
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		resp, err = c.httpClient.Do(req)
		if err == nil {
			break
		}
		if attempt == c.maxRetries {
			return nil, err
		}
		timer := time.NewTimer(c.retryWait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var providerErr ProviderError
	if err := json.Unmarshal(data, &providerErr); err != nil {
		providerErr.Message = strings.TrimSpace(string(data))
	}
	providerErr.StatusCode = resp.StatusCode
	providerErr.Retryable = resp.StatusCode >= 500
	if providerErr.Message == "" {
		providerErr.Message = resp.Status
	}
	return nil, providerErr
}

func normalizeEndpoint(baseURL, endpoint string) string {
	if endpoint == "" {
		return endpoint
	}
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return endpoint
	}
	scheme := "http"
	if parsed, err := url.Parse(baseURL); err == nil && parsed.Scheme != "" {
		scheme = parsed.Scheme
	}
	return scheme + "://" + strings.TrimPrefix(endpoint, "//")
}

func copyHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(headers))
	for key, value := range headers {
		out[key] = value
	}
	return out
}

func pathDir(name string) string {
	index := strings.LastIndex(name, string(os.PathSeparator))
	if index < 0 {
		return "."
	}
	return name[:index]
}
