package client

import (
	"context"
	"fmt"
	"net/http"
)

type HealthResponse struct {
	Status string `json:"status"`
}

type PaginationInfo struct {
	Page        int  `json:"page"`
	PageSize    int  `json:"pageSize"`
	TotalItems  int  `json:"totalItems"`
	TotalPages  int  `json:"totalPages"`
	HasNextPage bool `json:"hasNextPage"`
}

type ListSandboxesResponse struct {
	Items      []OSSandboxInfo `json:"items"`
	Pagination PaginationInfo  `json:"pagination"`
}

type OpenAPIDocument struct {
	OpenAPI string      `json:"openapi"`
	Info    OpenAPIInfo `json:"info"`
}

type OpenAPIInfo struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version"`
}

func (c *Client) Health(ctx context.Context) (HealthResponse, error) {
	var out HealthResponse
	err := c.doJSON(ctx, http.MethodGet, "/health", nil, &out, nil)
	return out, err
}

func (c *Client) OpenAPI(ctx context.Context) (OpenAPIDocument, error) {
	var out OpenAPIDocument
	err := c.doJSON(ctx, http.MethodGet, "/openapi.json", nil, &out, nil)
	return out, err
}

func (c *Client) ListSandboxes(ctx context.Context, page, pageSize int) (ListSandboxesResponse, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 1
	}
	var out ListSandboxesResponse
	reqPath := fmt.Sprintf("/v1/sandboxes?page=%d&pageSize=%d", page, pageSize)
	err := c.doJSON(ctx, http.MethodGet, reqPath, nil, &out, nil)
	return out, err
}
