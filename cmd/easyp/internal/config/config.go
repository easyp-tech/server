package config

import (
	"net/netip"

	"github.com/easyp-tech/server/cmd/easyp/internal/config/cachetype"
)

type Config struct {
	Listen netip.AddrPort `json:"listen"`
	Domain string         `json:"domain"`
	TLS    TLSConfig      `json:"tls"`
	Cache  Cache          `json:"cache"`
	Proxy  Proxy          `json:"proxy"`
	Local  LocalGit       `json:"local"`
	Log    LogConfig      `json:"log"`
}

type LogConfig struct {
	Level string `json:"level"`
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
}

type BitBucketRepo struct {
	Repo        Repo   `json:"repo"`
	User        string `json:"user"`
	AccessToken string `json:"token"`
	BaseURL     URL    `json:"url"`
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
}
