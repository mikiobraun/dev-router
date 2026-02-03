package scanner

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Project struct {
	Name    string
	Port    int
	Path    string
	Enabled bool
}

type ScanResult struct {
	Projects []Project
	Warnings []string
}

type serviceConfig struct {
	Name    string `yaml:"name"`
	Port    int    `yaml:"port"`
	Enabled *bool  `yaml:"enabled"`
}

type devConfig struct {
	// Single service format
	Port    int    `yaml:"port"`
	Name    string `yaml:"name"`
	Enabled *bool  `yaml:"enabled"`
	// Multi-service format
	Services []serviceConfig `yaml:"services"`
}

func Scan(projectsDir string) (*ScanResult, error) {
	result := &ScanResult{}

	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirPath := filepath.Join(projectsDir, entry.Name())

		// Check for .git directory
		gitPath := filepath.Join(dirPath, ".git")
		if _, err := os.Stat(gitPath); os.IsNotExist(err) {
			continue
		}

		// Check for dev.yaml
		devYamlPath := filepath.Join(dirPath, "dev.yaml")
		data, err := os.ReadFile(devYamlPath)
		if err != nil {
			continue // No dev.yaml, skip
		}

		var devCfg devConfig
		if err := yaml.Unmarshal(data, &devCfg); err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: malformed YAML: %v", entry.Name(), err))
			continue
		}

		// Multi-service format
		if len(devCfg.Services) > 0 {
			for _, svc := range devCfg.Services {
				enabled := true
				if svc.Enabled != nil {
					enabled = *svc.Enabled
				}
				result.Projects = append(result.Projects, Project{
					Name:    svc.Name,
					Port:    svc.Port,
					Path:    dirPath,
					Enabled: enabled,
				})
			}
			continue
		}

		// Single service format
		name := entry.Name()
		if devCfg.Name != "" {
			name = devCfg.Name
		}

		enabled := true
		if devCfg.Enabled != nil {
			enabled = *devCfg.Enabled
		}

		result.Projects = append(result.Projects, Project{
			Name:    name,
			Port:    devCfg.Port,
			Path:    dirPath,
			Enabled: enabled,
		})
	}

	// Check for duplicate ports
	portUsers := make(map[int][]string)
	for _, p := range result.Projects {
		if p.Enabled {
			portUsers[p.Port] = append(portUsers[p.Port], p.Name)
		}
	}
	for port, names := range portUsers {
		if len(names) > 1 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("port %d used by multiple services: %v", port, names))
		}
	}

	return result, nil
}
