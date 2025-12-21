package config

import (
	"encoding/json"
	"fmt"
)

type GlobalConfig struct {
	Clusters []ClusterConfig `json:"clusters"`
}

type ClusterConfig struct {
	Name      string `json:"name"`
	APIServer string `json:"apiServer"`
	CACert    string `json:"caCert"`
	Token     string `json:"token"`
}

func (c *GlobalConfig) Validate() error {
	if len(c.Clusters) == 0 {
		return fmt.Errorf("at least one cluster must be configured")
	}
	for i, cluster := range c.Clusters {
		if err := cluster.Validate(); err != nil {
			return fmt.Errorf("cluster[%d]: %w", i, err)
		}
	}
	return nil
}

func (c *ClusterConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if c.APIServer == "" {
		return fmt.Errorf("apiServer is required")
	}
	if c.CACert == "" {
		return fmt.Errorf("caCert is required")
	}
	if c.Token == "" {
		return fmt.Errorf("token is required")
	}
	return nil
}

func ParseConfig(data []byte) (*GlobalConfig, error) {
	var config GlobalConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &config, nil
}
