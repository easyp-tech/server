package config

import (
	"net/netip"
)

type Config struct {
	Listen  netip.AddrPort `json:"listen"`
	Domain  string         `json:"domain"`
	Storage string         `json:"storage"`
	TLS     TLSConfig      `json:"tls"`
	Proxy   Proxy          `json:"proxy"`
}

type TLSConfig struct {
	CertFile   string `json:"cert"`
	KeyFile    string `json:"key"`
	CACertFile string `json:"ca"`
}

type Proxy struct {
	Cache  string `json:"cache"`
	Github Github `json:"github"`
}

type Github struct {
	Repos []GithubRepo `json:"repos"`
}

type GithubRepo struct {
	Owner       string   `json:"owner"`
	Name        string   `json:"name"`
	Paths       []string `json:"paths"`
	AccessToken string   `json:"token"`
}
