package util

import (
	"os"
	"strings"
)

func EnvValue(name, fallback string) string {
	v, ok := os.LookupEnv(name)
	if !ok {
		return fallback
	}

	if strings.HasPrefix(v, `file:`) {
		b, err := os.ReadFile(strings.TrimPrefix(v, `file:`))
		if err != nil {
			return fallback
		}
		return string(b)
	}

	return v

}
