package main

import (
	"flag"
	"log"
	"os"

	"github.com/umegbewe/dhcpd/internal/config"
	"github.com/umegbewe/dhcpd/pkg/dhcp"
)

func main() {
	configFile := flag.String("conf", "conf.yaml", "Path to the configuration file")

	if envConfig := os.Getenv("DHCP_CONFIG_PATH"); envConfig != "" {
		*configFile = envConfig
	}

	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	server, err := dhcp.InitServer(cfg)
	if err != nil {
		log.Fatalf("Failed to create DHCP server: %v", err)
	}

	err = server.Start()
	if err != nil {
		log.Fatalf("Failed to start DHCP server: %v", err)
	}

}
