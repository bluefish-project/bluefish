package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"

	"github.com/bluefish-project/bluefish/rvfs"
)

type Config struct {
	Endpoint string `yaml:"endpoint"`
	User     string `yaml:"user"`
	Pass     string `yaml:"pass"`
	Insecure bool   `yaml:"insecure"`
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: bfui CONFIG_FILE")
		os.Exit(1)
	}

	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Printf("Error reading config: %v\n", err)
		os.Exit(1)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		fmt.Printf("Error parsing config: %v\n", err)
		os.Exit(1)
	}

	vfs, err := rvfs.NewVFS(cfg.Endpoint, cfg.User, cfg.Pass, cfg.Insecure)
	if err != nil {
		fmt.Printf("Error creating VFS: %v\n", err)
		os.Exit(1)
	}
	defer vfs.Sync()

	m := NewModel(vfs)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
