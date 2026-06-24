package config

import (
	"net/netip"
	"time"

	"github.com/easyp-tech/server/cmd/easyp/internal/config/cachetype"
)

type Config struct {
	Listen  netip.AddrPort `json:"listen"`
	Domain  string         `json:"domain"`
	TLS     TLSConfig      `json:"tls"`
	Cache   Cache          `json:"cache"`
	Proxy   Proxy          `json:"proxy"`
	Local   LocalGit       `json:"local"`
	Log     LogConfig      `json:"log"`
	Connect Connect        `json:"connect"`
}

type LogConfig struct {
	Level     string `json:"level"`
	Format    string `json:"format,omitempty"`
	AddSource bool   `json:"add_source,omitempty"`
}

type TLSConfig struct {
	CertFile   string `json:"cert"`
	KeyFile    string `json:"key"`
	CACertFile string `json:"ca"`
}

type LocalGit struct {
	Storage string `json:"storage"`
	Repos   []Repo `json:"repo"`
}

type Proxy struct {
	Github    []GithubRepo    `json:"github"`
	BitBucket []BitBucketRepo `json:"bitbucket"`
}

type GithubRepo struct {
	Repo        Repo   `json:"repo"`
	AccessToken string `json:"token"`
	Timeout     int    `json:"timeout"`
	BodyLimit   int64  `json:"bodyLimit"`
}

type BitBucketRepo struct {
	Repo        Repo   `json:"repo"`
	User        string `json:"user"`
	AccessToken string `json:"token"`
	BaseURL     URL    `json:"url"`
	Timeout     int    `json:"timeout"`
	BodyLimit   int64  `json:"bodyLimit"`
}

type Repo struct {
	Owner    string   `json:"owner"`
	Name     string   `json:"name"`
	Prefixes []string `json:"prefix"`
	Paths    []string `json:"path"`
}

type Cache struct {
	Type        cachetype.Type `json:"type"`
	Local       CacheLocal     `json:"local"`
	Artifactory Artifactory    `json:"artifactory"`
}

type CacheLocal struct {
	Dir string `json:"directory"`
}

type Artifactory struct {
	User        string `json:"user"`
	AccessToken string `json:"token"`
	BaseURL     URL    `json:"url"`
	Timeout     int    `json:"timeout"`
	BodyLimit   int64  `json:"bodyLimit"`
}

// Connect configures the buf v1 registry commit-id resolution behavior in
// internal/connect. All fields default sensibly (WithDefaults) so the proxy
// behaves as intended with no connect: block in the config file.
type Connect struct {
	Prewarm PrewarmConfig `json:"prewarm"`
	Probe   ProbeConfig   `json:"probe"`
}

// PrewarmConfig controls startup HEAD pre-warming: the proxy resolves the
// current HEAD commit of every configured module at startup so that clients
// caching a current HEAD sha hit the commit map without a prior in-session
// GetCommits.
type PrewarmConfig struct {
	// Enabled is a pointer so we can distinguish "unset" (default true) from
	// an explicit false. Set enabled: false to disable.
	Enabled        *bool         `json:"enabled"`
	PerCallTimeout time.Duration `json:"per_call_timeout"`
}

// ProbeConfig controls the upstream sha probe used on a Download cache miss:
// the proxy asks each configured source whether it owns the requested sha and,
// on a hit, resolves the module. Bogus shas are negative-cached for NegativeTTL
// to bound repeat cost.
type ProbeConfig struct {
	// Enabled is a pointer so we can distinguish "unset" (default true) from
	// an explicit false. Set enabled: false to disable.
	Enabled        *bool         `json:"enabled"`
	NegativeTTL    time.Duration `json:"negative_ttl"`
	PerCallTimeout time.Duration `json:"per_call_timeout"`
}

// WithDefaults returns a copy of the Connect config with zero-value fields
// replaced by the documented defaults. Called once at startup after the config
// is loaded.
func (c Connect) WithDefaults() Connect {
	out := c
	if out.Prewarm.Enabled == nil {
		t := true
		out.Prewarm.Enabled = &t
	}
	if out.Prewarm.PerCallTimeout == 0 {
		out.Prewarm.PerCallTimeout = 10 * time.Second
	}
	if out.Probe.Enabled == nil {
		t := true
		out.Probe.Enabled = &t
	}
	if out.Probe.PerCallTimeout == 0 {
		out.Probe.PerCallTimeout = 8 * time.Second
	}
	if out.Probe.NegativeTTL == 0 {
		out.Probe.NegativeTTL = 5 * time.Minute
	}
	return out
}