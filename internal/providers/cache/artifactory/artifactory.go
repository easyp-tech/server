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

	"golang.org/x/exp/slog"

	"github.com/easyp-tech/server/internal/providers/content"
)

var ErrUnexpected = errors.New("unexpected")

func New(
	log *slog.Logger,
	baseURL string,
	user string,
	password string,
) artifactory {
	return artifactory{
		log:      log,
		baseURL:  baseURL,
		user:     user,
		password: password,
	}
}

type artifactory struct {
	log      *slog.Logger
	baseURL  string
	user     string
	password string
}

func (c artifactory) Get(
	ctx context.Context,
	owner string,
	repoName string,
	commit string,
	configHash string,
) ([]content.File, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		strings.Join([]string{c.baseURL, owner, repoName, configHash, commit + ".json"}, "/"),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	req.SetBasicAuth(c.user, c.password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting %q: %w", req.URL.String(), err)
	}

	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getting %q: response %d: %w", req.URL.String(), resp.StatusCode, ErrUnexpected)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", req.URL.String(), err)
	}

	var out []content.File

	if err = json.Unmarshal(data, &out); err != nil { //nolint:musttag
		return nil, fmt.Errorf("decoding %q: %w", req.URL.String(), err)
	}

	return out, nil
}

func (c artifactory) Put(ctx context.Context, owner, repoName, commit, configHash string, in []content.File) error {
	var buf bytes.Buffer

	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(in); err != nil { //nolint:musttag
		return fmt.Errorf("encoding: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		strings.Join([]string{c.baseURL, owner, repoName, configHash, commit + ".json"}, "/"),
		&buf,
	)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}

	req.SetBasicAuth(c.user, c.password)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("putting %q: %w", req.URL.String(), err)
	}

	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK && resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("putting %q: response %d: %w", req.URL.String(), resp.StatusCode, ErrUnexpected)
	}

	return nil
}
