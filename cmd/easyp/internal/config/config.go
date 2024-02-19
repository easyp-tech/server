package config

import (
	"net/netip"
)

type Config struct {
	Listen  netip.AddrPort `json:"listen"`
	Domain  string         `json:"domain"`
	Storage string         `json:"storage"`
	TLS     TLSConfig      `json:"tls"`
}

type TLSConfig struct {
	CertFile   string `json:"cert"`
	KeyFile    string `json:"key"`
	CACertFile string `json:"ca"`
}
