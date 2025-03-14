package config

import (
	"fmt"
	"os"

	"github.com/mcuadros/go-defaults"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Server   Server   `yaml:"server"`
	Database Database `yaml:"database"`
	Logging  Logging  `yaml:"logging"`
	Metrics  Metrics  `yaml:"metrics"`
}

type Server struct {
	IPStart                string   `yaml:"ip_start"`
	IPEnd                  string   `yaml:"ip_end"`
	SubnetMask             string   `yaml:"subnet_mask"`
	LeaseTime              int      `yaml:"lease_time"`
	Gateway                string   `yaml:"gateway"`
	ServerIP               string   `yaml:"server_ip"`
	DNSServers             []string `yaml:"dns_servers"`
	TFTPServerName         string   `yaml:"tftp_server_name"`
	BootFileName           string   `yaml:"boot_file_name"`
	Interface              string   `yaml:"interface"`
	Port                   int      `yaml:"port" default:"67"`
	CleanupExpiredInterval int      `yaml:"cleanup_expired_interval" default:"120"`
	ARPCheck               bool     `yaml:"arp_check" default:"true"`
}

type Logging struct {
	Level string `yaml:"level"`
}

type Metrics struct {
	Enabled       bool   `yaml:"enabled" default:"true"`
	ListenAddress string `yaml:"listen_address" default:":9100"`
}

type Database struct {
	Type  string      `yaml:"type" default:"bolt"`
	Bolt  BoltConfig  `yaml:"bolt"`
	Redis RedisConfig `yaml:"redis"`
}

type BoltConfig struct {
	Path string `yaml:"path"`
}

type RedisConfig struct {
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db" default:"0"`
}

func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read configuration file: %v", err)
	}

	cfg := &Config{}
	defaults.SetDefaults(cfg)
	err = yaml.Unmarshal(data, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse configuration YAML: %v", err)
	}

	return cfg, nil
}
