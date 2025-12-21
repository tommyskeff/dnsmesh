package config

import (
	"testing"
)

func TestParseConfig_Valid(t *testing.T) {
	jsonData := `{
		"clusters": [
			{
				"name": "na-prod",
				"apiServer": "https://10.20.1.1:6443",
				"caCert": "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----",
				"token": "test-token"
			}
		]
	}`

	cfg, err := ParseConfig([]byte(jsonData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Clusters) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(cfg.Clusters))
	}
	if cfg.Clusters[0].Name != "na-prod" {
		t.Errorf("expected cluster name 'na-prod', got '%s'", cfg.Clusters[0].Name)
	}
}

func TestParseConfig_MultipleClusters(t *testing.T) {
	jsonData := `{
		"clusters": [
			{"name": "na-prod", "apiServer": "https://10.20.1.1:6443", "caCert": "cert1", "token": "token1"},
			{"name": "eu-prod", "apiServer": "https://10.20.2.1:6443", "caCert": "cert2", "token": "token2"},
			{"name": "asia-prod", "apiServer": "https://10.20.3.1:6443", "caCert": "cert3", "token": "token3"}
		]
	}`

	cfg, err := ParseConfig([]byte(jsonData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Clusters) != 3 {
		t.Errorf("expected 3 clusters, got %d", len(cfg.Clusters))
	}
}

func TestParseConfig_MissingClusters(t *testing.T) {
	jsonData := `{
		"clusters": []
	}`

	_, err := ParseConfig([]byte(jsonData))
	if err == nil {
		t.Error("expected error for missing clusters")
	}
}

func TestParseConfig_InvalidCluster(t *testing.T) {
	testCases := []struct {
		name string
		json string
	}{
		{
			name: "missing name",
			json: `{"clusters": [{"apiServer": "https://test:6443", "caCert": "cert", "token": "token"}]}`,
		},
		{
			name: "missing apiServer",
			json: `{"clusters": [{"name": "test", "caCert": "cert", "token": "token"}]}`,
		},
		{
			name: "missing caCert",
			json: `{"clusters": [{"name": "test", "apiServer": "https://test:6443", "token": "token"}]}`,
		},
		{
			name: "missing token",
			json: `{"clusters": [{"name": "test", "apiServer": "https://test:6443", "caCert": "cert"}]}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tc.json))
			if err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestParseConfig_InvalidJSON(t *testing.T) {
	_, err := ParseConfig([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
