package root

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type serverFileConfig struct {
	Listen          string   `yaml:"listen"`
	Domain          string   `yaml:"domain"`
	SubdomainPrefix string   `yaml:"subdomain_prefix"`
	Mode            string   `yaml:"mode"`
	TLSCert         string   `yaml:"tls_cert"`
	TLSKey          string   `yaml:"tls_key"`
	Trusted         []string `yaml:"trusted_proxies"`
	TrustedHops     int      `yaml:"trusted_hops"`
	Authorized      string   `yaml:"authorized"`
	State           string   `yaml:"state"`
	AutoReserve     *bool    `yaml:"reserve"`
	AllowRandom     *bool    `yaml:"allow_random"`
	AdminSecret     string   `yaml:"admin_secret"`
}

func loadServerConfig(path string) (*serverFileConfig, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg serverFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
