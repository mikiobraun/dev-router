package main

import (
	"fmt"
	"os"
	"os/exec"

	"dev-router/internal/config"
	"dev-router/internal/generator"
	"dev-router/internal/scanner"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	switch cmd {
	case "generate":
		cmdGenerate()
	case "list":
		cmdList()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("dev-router - Automatic subdomain routing for local dev services")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  dev-router generate [--reload]  Scan projects and write Caddyfile")
	fmt.Println("  dev-router list                 Show discovered services")
	fmt.Println("  dev-router help                 Show this help")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --reload    Reload Caddy after generating (requires sudo)")
}

func loadConfig() *config.Config {
	cfg, err := config.Load(config.DefaultConfigPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		fmt.Fprintf(os.Stderr, "Expected config at: %s\n", config.DefaultConfigPath())
		os.Exit(1)
	}
	return cfg
}

func cmdGenerate() {
	// Check for --reload flag
	reload := false
	for _, arg := range os.Args[2:] {
		if arg == "--reload" {
			reload = true
		}
	}

	cfg := loadConfig()

	projects, err := scanner.Scan(cfg.ProjectsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning projects: %v\n", err)
		os.Exit(1)
	}

	content := generator.Generate(cfg, projects)

	if err := generator.Write(cfg, content); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing Caddyfile: %v\n", err)
		os.Exit(1)
	}

	// Count enabled projects
	enabled := 0
	for _, p := range projects {
		if p.Enabled {
			enabled++
		}
	}

	fmt.Printf("Generated %s with %d service(s)\n", cfg.CaddyfilePath, enabled)

	if reload {
		cmd := exec.Command("sudo", "systemctl", "reload", "caddy")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error reloading Caddy: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Caddy reloaded")
	}
}

func cmdList() {
	cfg := loadConfig()

	projects, err := scanner.Scan(cfg.ProjectsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error scanning projects: %v\n", err)
		os.Exit(1)
	}

	if len(projects) == 0 {
		fmt.Println("No services found")
		return
	}

	fmt.Printf("%-20s %-30s %s\n", "NAME", "URL", "PORT")
	fmt.Printf("%-20s %-30s %s\n", "----", "---", "----")

	for _, p := range projects {
		status := ""
		if !p.Enabled {
			status = " (disabled)"
		}
		url := fmt.Sprintf("https://%s.%s", p.Name, cfg.Domain)
		fmt.Printf("%-20s %-30s %d%s\n", p.Name, url, p.Port, status)
	}
}
