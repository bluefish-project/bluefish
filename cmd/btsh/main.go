package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"github.com/bluefish-project/bluefish/rvfs"
)

// Config holds connection configuration
type Config struct {
	Endpoint string `yaml:"endpoint"`
	User     string `yaml:"user"`
	Pass     string `yaml:"pass"`
	Insecure bool   `yaml:"insecure"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: btsh CONFIG_FILE")
		fmt.Println("Example: btsh config.yaml")
		os.Exit(1)
	}

	configPath := os.Args[1]

	if !strings.HasSuffix(configPath, ".yaml") && !strings.HasSuffix(configPath, ".yml") {
		fmt.Println("Usage: btsh CONFIG_FILE")
		fmt.Println("Example: btsh config.yaml")
		os.Exit(1)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("Error reading config: %v\n", err)
		os.Exit(1)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Printf("Error parsing config: %v\n", err)
		os.Exit(1)
	}

	if cfg.Endpoint == "" || cfg.User == "" || cfg.Pass == "" {
		fmt.Println("Config must include: endpoint, user, pass")
		os.Exit(1)
	}

	fmt.Printf("Connecting to %s...\n", cfg.Endpoint)
	vfs, err := rvfs.NewVFS(cfg.Endpoint, cfg.User, cfg.Pass, cfg.Insecure)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	defer vfs.Sync()

	nav := NewNavigator(vfs)
	history := NewHistory(os.ExpandEnv("$HOME/.btsh_history"))

	// Show initial status
	entries, _ := vfs.ListAll(nav.cwd)
	summary := getEntriesSummary(entries)
	fmt.Printf("%s  (%s)\n", nav.cwd, summary)
	fmt.Println("Type 'help' for commands")

	state := &shellState{
		nav:     nav,
		history: history,
	}

	m := newModel(state)
	p := tea.NewProgram(m, tea.WithoutCatchPanics())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
