package config

import (
	"net/netip"
)

type Config struct {
	Listen netip.AddrPort `json:"listen"`
	Domain string         `json:"domain"`
	TLS    TLSConfig      `json:"tls"`
	Cache  string         `json:"cache"`
	Proxy  Proxy          `json:"proxy"`
	Local  LocalGit       `json:"local"`
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
