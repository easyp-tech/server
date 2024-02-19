package config

import (
	"fmt"
	"os"

	"github.com/ghodss/yaml"
)

func ReadYaml[T any](fileName string) (T, error) {
	dst := new(T)

	data, err := os.ReadFile(fileName)
	if err != nil {
		return *dst, fmt.Errorf("reading %q: %w", fileName, err)
	}

	if err = yaml.Unmarshal(data, dst); err != nil {
		return *dst, fmt.Errorf("parsing %q: %w", fileName, err)
	}

	return *dst, nil
}
