package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDockerComposeGatewayPassesAIProvider(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "docker-compose.yml"))
	require.NoError(t, err)

	var compose struct {
		Services map[string]struct {
			Environment []string `yaml:"environment"`
		} `yaml:"services"`
	}
	require.NoError(t, yaml.Unmarshal(data, &compose))

	gatewayService, ok := compose.Services["gateway"]
	require.True(t, ok, "docker-compose.yml must define the gateway service")
	require.Contains(t, gatewayService.Environment, "AI_PROVIDER=${AI_PROVIDER:-mock}")
}
