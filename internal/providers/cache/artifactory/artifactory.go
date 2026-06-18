package artifactory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"log/slog"

	connectpkg "github.com/easyp-tech/server/internal/connect"
	"github.com/easyp-tech/server/internal/providers/content"
)

const defaultBodyLimit = 50 * 1 << 20 // 50MB

var (
	ErrUnexpected = errors.New("unexpected")
	testFilePath   = "buf-proxy-connection-test.json"
)

func New(
	log *slog.Logger,
	baseURL string,
	user string,
	password string,
	timeout time.Duration,
	bodyLimit int64,
) artifactory {
	if log == nil {
		log = slog.Default()
	}
	if bodyLimit <= 0 {
		bodyLimit = defaultBodyLimit
	}
	return artifactory{
		log:       log,
		baseURL:   baseURL,
		user:      user,
		password:  password,
		client:    http.Client{Timeout: timeout},
		bodyLimit: bodyLimit,
	}
}

type artifactory struct {
	log       *slog.Logger
	baseURL   string
	user      string
	password  string
	client    http.Client
	bodyLimit int64
}

func (c artifactory) Get(
	ctx context.Context,
	owner string,
	repoName string,
	commit string,
	configHash string,
) ([]content.File, error) {
	reqID := connectpkg.RequestIDFrom(ctx)
	url := strings.Join([]string{c.baseURL, owner, repoName, configHash, commit + ".json"}, "/")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.SetBasicAuth(c.user, c.password)

	start := time.Now()
	c.log.DebugContext(ctx, "cache Get start",
		slog.String("cache_type", "artifactory"),
		slog.String("url", url),
		slog.String("request_id", reqID),
	)
	resp, err := c.client.Do(req)
	dur := time.Since(start)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			c.log.LogAttrs(ctx, slog.LevelDebug, "cache Get cancelled",
				slog.String("cache_type", "artifactory"),
				slog.String("url", url),
				slog.String("request_id", reqID),
				slog.Duration("duration", dur),
			)
		} else {
			c.log.LogAttrs(ctx, slog.LevelDebug, "cache Get failed",
				slog.String("cache_type", "artifactory"),
				slog.String("url", url),
				slog.String("request_id", reqID),
				slog.Duration("duration", dur),
				slog.String("error", err.Error()),
			)
		}
		return nil, fmt.Errorf("getting %q: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		c.log.LogAttrs(ctx, slog.LevelDebug, "cache Get miss",
			slog.String("cache_type", "artifactory"),
			slog.String("url", url),
			slog.String("request_id", reqID),
			slog.Duration("duration", dur),
			slog.Int("status", resp.StatusCode),
		)
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		c.log.LogAttrs(ctx, slog.LevelDebug, "cache Get unexpected status",
			slog.String("cache_type", "artifactory"),
			slog.String("url", url),
			slog.String("request_id", reqID),
			slog.Duration("duration", dur),
			slog.Int("status", resp.StatusCode),
		)
		return nil, fmt.Errorf("getting %q: response %d: %w", url, resp.StatusCode, ErrUnexpected)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, c.bodyLimit))
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", url, err)
	}

	var out []content.File
	if err = json.Unmarshal(data, &out); err != nil { //nolint:musttag
		return nil, fmt.Errorf("decoding %q: %w", url, err)
	}

	c.log.LogAttrs(ctx, slog.LevelDebug, "cache Get hit",
		slog.String("cache_type", "artifactory"),
		slog.String("url", url),
		slog.String("request_id", reqID),
		slog.Duration("duration", dur),
		slog.Int("files", len(out)),
	)

	return out, nil
}

func (c artifactory) Put(ctx context.Context, owner, repoName, commit, configHash string, in []content.File) error {
	reqID := connectpkg.RequestIDFrom(ctx)
	url := strings.Join([]string{c.baseURL, owner, repoName, configHash, commit + ".json"}, "/")

	var buf bytes.Buffer

	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(in); err != nil { //nolint:musttag
		return fmt.Errorf("encoding: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, &buf)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.SetBasicAuth(c.user, c.password)

	start := time.Now()
	c.log.DebugContext(ctx, "cache Put start",
		slog.String("cache_type", "artifactory"),
		slog.String("url", url),
		slog.String("request_id", reqID),
	)
	resp, err := c.client.Do(req)
	dur := time.Since(start)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			c.log.LogAttrs(ctx, slog.LevelDebug, "cache Put cancelled",
				slog.String("cache_type", "artifactory"),
				slog.String("url", url),
				slog.String("request_id", reqID),
				slog.Duration("duration", dur),
			)
		} else {
			c.log.LogAttrs(ctx, slog.LevelDebug, "cache Put failed",
				slog.String("cache_type", "artifactory"),
				slog.String("url", url),
				slog.String("request_id", reqID),
				slog.Duration("duration", dur),
				slog.String("error", err.Error()),
			)
		}
		return fmt.Errorf("putting %q: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		c.log.LogAttrs(ctx, slog.LevelDebug, "cache Put unexpected status",
			slog.String("cache_type", "artifactory"),
			slog.String("url", url),
			slog.String("request_id", reqID),
			slog.Duration("duration", dur),
			slog.Int("status", resp.StatusCode),
		)
		return fmt.Errorf("putting %q: response %d: %w", url, resp.StatusCode, ErrUnexpected)
	}

	c.log.LogAttrs(ctx, slog.LevelDebug, "cache Put completed",
		slog.String("cache_type", "artifactory"),
		slog.String("url", url),
		slog.String("request_id", reqID),
		slog.Duration("duration", dur),
		slog.Int("files", len(in)),
	)

	return nil
}

func (c artifactory) CheckWriteAccess(ctx context.Context) error {
	url := c.baseURL + testFilePath
	testContent := []byte(`{"status": "test"}`)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		url,
		bytes.NewReader(testContent),
	)
	if err != nil {
		return fmt.Errorf("building test write request: %w", err)
	}

	req.SetBasicAuth(c.user, c.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("test write request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("test write failed: status %d, response: %q",
			resp.StatusCode, string(body))
	}

	req, err = http.NewRequestWithContext(
		ctx,
		http.MethodDelete,
		url,
		nil,
	)
	if err != nil {
		return fmt.Errorf("building test delete request: %w", err)
	}

	req.SetBasicAuth(c.user, c.password)
	resp, err = c.client.Do(req)
	if err != nil {
		return fmt.Errorf("test delete request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("test delete failed: status %d, response: %q",
			resp.StatusCode, string(body))
	}

	return nil
}