package config

import "strings"

type Reader struct {
	path string
}

func NewReader(path string) Reader {
	return Reader{path: strings.TrimSpace(path)}
}

func (r Reader) Path() string {
	if r.path == "" {
		return defaultConfigPath()
	}
	return r.path
}

func (r Reader) Read() (Config, error) {
	return Load(r.Path())
}
