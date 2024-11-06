package config

import "git.asdf.cafe/abs3nt/gunner"

type Config struct {
	TargetIP   string `yaml:"target_ip"`
	TargetPort string `yaml:"target_port" default:"5000"`
	HostIP     string `yaml:"host_ip"`
	HostPort   int    `yaml:"host_port" default:"8080"`
}

func NewConfig() *Config {
	c := &Config{}
	gunner.LoadApp(c, "fbisender")
	return c
}
