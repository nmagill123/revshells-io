package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const configFileName = ".rsctl"

type fileConfig struct {
	Server string
	Token  string
}

func configPath() string {
	if _, err := os.Stat(configFileName); err == nil {
		p, _ := filepath.Abs(configFileName)
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return configFileName
	}
	return filepath.Join(home, configFileName)
}

func loadFileConfig() fileConfig {
	var c fileConfig
	f, err := os.Open(configPath())
	if err != nil {
		return c
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(strings.Trim(v, `"`))
		switch strings.ToLower(k) {
		case "server", "rsd_server":
			c.Server = v
		case "token", "rsd_token":
			c.Token = v
		}
	}
	return c
}

func saveFileConfig(server, token string) error {
	path := configPath()
	content := fmt.Sprintf("# rsd operator config (6h token)\nserver=%s\ntoken=%s\n", server, token)
	return os.WriteFile(path, []byte(content), 0600)
}

func resolveConfig(flagServer, flagToken string) (server, token string) {
	server = strings.TrimSpace(os.Getenv("RSD_SERVER"))
	token = strings.TrimSpace(os.Getenv("RSD_TOKEN"))
	if server == "" || token == "" {
		fc := loadFileConfig()
		if server == "" {
			server = fc.Server
		}
		if token == "" {
			token = fc.Token
		}
	}
	if flagServer != "" {
		server = flagServer
	}
	if flagToken != "" {
		token = flagToken
	}
	return strings.TrimRight(server, "/"), token
}
