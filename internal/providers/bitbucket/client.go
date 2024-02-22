package bitbucket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"text/template"

	"golang.org/x/exp/slog"
)

type client struct {
	log    *slog.Logger
	client httpClient
}

func connect(log *slog.Logger, user User, token Password, baseURL string) client {
	return client{
		log: log,
		client: httpClient{
			basePath: baseURL,
			user:     string(user),
			password: string(token),
		},
	}
}

var ErrUnexpected = errors.New("unexpected")

type (
	paramsMap map[string]string
	qeryMap   map[string]string
)

func httpGetJSON[T any](
	ctx context.Context,
	cln httpClient,
	path *template.Template,
	params paramsMap,
	query qeryMap,
) (T, error) {
	var out T

	body, err := cln.get(ctx, path, params, query)
	if err != nil {
		return out, fmt.Errorf("requesting: %w", err)
	}

	if err = json.Unmarshal(body, &out); err != nil {
		return out, fmt.Errorf("decoding: %w", err)
	}

	return out, nil
}

//nolint:gochecknoglobals
var (
	tmplGetDefaultBranch = tmplBuild("/branches/default")
	tmplGetFilesList     = tmplBuild("/files")
	tmplGetFileContent   = tmplBuild("/raw/{{.name}}")
)

type httpClient struct {
	basePath string
	user     string
	password string
}

func (c httpClient) get(
	ctx context.Context,
	path *template.Template,
	params paramsMap,
	query qeryMap,
) ([]byte, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.basePath+tmplExec(path, params),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	req.URL.RawQuery = buildQuery(req.URL.Query(), query)
	req.SetBasicAuth(c.user, c.password)
	req.Header.Add("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting %q: %w", req.URL.String(), err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getting %q: response %d: %w", req.URL.String(), resp.StatusCode, ErrUnexpected)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading %q: %w", req.URL.String(), err)
	}

	return b, nil
}

func tmplBuild(tmpl string) *template.Template {
	return template.Must(template.New("").Parse(tmpl))
}

func tmplExec(tmpl *template.Template, params map[string]string) string {
	var buf bytes.Buffer

	if err := tmpl.Execute(&buf, params); err != nil {
		panic(err)
	}

	return buf.String()
}

func buildQuery(query url.Values, params map[string]string) string {
	for k, v := range params {
		query.Set(k, v)
	}

	return query.Encode()
}
