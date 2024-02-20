package config

import (
	"net/netip"
)

type Config struct {
	Listen  netip.AddrPort `json:"listen"`
	Domain  string         `json:"domain"`
	Storage string         `json:"storage"`
}
