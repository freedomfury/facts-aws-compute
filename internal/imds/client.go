package imds

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/smithy-go"

	"github.com/facts/facts-aws-compute/internal/config"
	"github.com/facts/facts-aws-compute/internal/output"
)

// Client wraps the AWS IMDS client, enforcing IMDSv2-only access.
type Client struct {
	inner *imds.Client
}

// New creates an IMDS client. The caller must pass an imds.Client
// already configured (typically via config.LoadDefaultConfig).
func New(c *imds.Client) *Client {
	return &Client{inner: c}
}

// Get fetches a single metadata path. Returns ("", nil) on 404.
func (c *Client) Get(ctx context.Context, path string) (string, error) {
	// Ensure we don't hang indefinitely — if caller already provided a
	// deadline, honor it; otherwise use a sensible default timeout.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		t := config.IMDSTimeout(3 * time.Second)
		output.Debugf("applying IMDS timeout: %s", t)
		ctx, cancel = context.WithTimeout(ctx, t)
		defer cancel()
	}

	output.Debugf("IMDS GET %s", path)
	out, err := c.inner.GetMetadata(ctx, &imds.GetMetadataInput{
		Path: path,
	})
	if err != nil {
		if isNotFound(err) {
			output.Debugf("IMDS GET %s -> 404", path)
			return "", nil
		}
		output.Debugf("IMDS GET %s -> error: %v", path, err)
		return "", fmt.Errorf("IMDS GET %s: %w", path, err)
	}
	defer out.Content.Close()

	data, err := io.ReadAll(out.Content)
	if err != nil {
		return "", fmt.Errorf("IMDS read %s: %w", path, err)
	}
	output.Debugf("IMDS GET %s -> %d bytes", path, len(data))
	return strings.TrimSpace(string(data)), nil
}

// isNotFound returns true when the error represents a 404/not found
// condition from the IMDS or AWS SDK.
func isNotFound(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		if code == "NotFound" || code == "NotFoundException" {
			return true
		}
	}
	// Fallback to string inspection for older SDKs.
	if strings.Contains(err.Error(), "StatusCode: 404") ||
		strings.Contains(err.Error(), "status code: 404") ||
		strings.Contains(err.Error(), "404") {
		return true
	}
	return false
}

// GetRequired fetches a metadata path and returns a fatal-style error if
// the value is empty or absent.
func (c *Client) GetRequired(ctx context.Context, path string) (string, error) {
	v, err := c.Get(ctx, path)
	if err != nil {
		return "", err
	}
	if v == "" {
		return "", fmt.Errorf("IMDS path %s returned empty value", path)
	}
	return v, nil
}

// List fetches a directory-style metadata path and returns the list of keys.
// Keys ending in "/" are subdirectories. Returns (nil, nil) on 404.
func (c *Client) List(ctx context.Context, path string) ([]string, error) {
	raw, err := c.Get(ctx, path)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	lines := strings.Split(raw, "\n")
	var result []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			result = append(result, l)
		}
	}
	return result, nil
}
