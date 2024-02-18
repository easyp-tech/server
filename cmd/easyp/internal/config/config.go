package config

type (
	//nolint:tagliatelle
	Config struct {
		Server server  `json:"server"`
		Store  storage `json:"storage"`
	}
	server struct {
		External external `json:"external"`
	}
	ports struct {
		Connect uint16 `json:"connect"`
		Metric  uint16 `json:"metric"`
	}
	external struct {
		Domain string `json:"domain"`
		Host   string `json:"host"`
		Port   ports  `json:"port"`
	}
	storage struct {
		Root string `json:"root"`
	}
)
