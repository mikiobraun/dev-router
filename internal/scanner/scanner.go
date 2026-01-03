package scanner

import (
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

func Scan(projectsDir string) ([]Project, error) {
	var projects []Project

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
			continue // Invalid yaml, skip
		}

		// Multi-service format
		if len(devCfg.Services) > 0 {
			for _, svc := range devCfg.Services {
				enabled := true
				if svc.Enabled != nil {
					enabled = *svc.Enabled
				}
				projects = append(projects, Project{
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

		projects = append(projects, Project{
			Name:    name,
			Port:    devCfg.Port,
			Path:    dirPath,
			Enabled: enabled,
		})
	}

	return projects, nil
}
