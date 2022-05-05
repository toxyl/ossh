package main

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/viper"
)

type SyncNode struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
}

type Config struct {
	PathData         string   `mapstructure:"path_data"`
	PathFingerprints string   `mapstructure:"path_fingerprints"`
	PathPasswords    string   `mapstructure:"path_passwords"`
	PathUsers        string   `mapstructure:"path_users"`
	PathHosts        string   `mapstructure:"path_hosts"`
	PathCommands     string   `mapstructure:"path_commands"`
	PathWebinterface string   `mapstructure:"path_webinterface"`
	PathCaptures     string   `mapstructure:"path_captures"`
	PathFFS          string   `mapstructure:"path_ffs"`
	HostName         string   `mapstructure:"host_name"`
	Version          string   `mapstructure:"version"`
	IPWhitelist      []string `mapstructure:"ip_whitelist"`
	Host             string   `mapstructure:"host"`
	Port             uint     `mapstructure:"port"`
	MaxIdleTimeout   uint     `mapstructure:"max_idle"`
	InputDelay       uint     `mapstructure:"input_delay"`
	Ratelimit        float64  `mapstructure:"ratelimit"`
	Webinterface     struct {
		Host     string `mapstructure:"host"`
		Port     uint   `mapstructure:"port"`
		CertFile string `mapstructure:"cert_file"`
		KeyFile  string `mapstructure:"key_file"`
	} `mapstructure:"webinterface"`
	Sync struct {
		Interval int        `mapstructure:"interval"`
		Nodes    []SyncNode `mapstructure:"nodes"`
	} `mapstructure:"sync"`
	Commands struct {
		Rewriters        [][]string `mapstructure:"rewriters"`
		Simple           [][]string `mapstructure:"simple"`
		Exit             []string   `mapstructure:"exit"`
		PermissionDenied []string   `mapstructure:"permission_denied"`
		DiskError        []string   `mapstructure:"disk_error"`
		CommandNotFound  []string   `mapstructure:"command_not_found"`
		FileNotFound     []string   `mapstructure:"file_not_found"`
		NotImplemented   []string   `mapstructure:"not_implemented"`
	} `mapstructure:"commands"`
}

var cfgFile string
var Conf Config

func isIPWhitelisted(ip string) bool {
	for _, wip := range Conf.IPWhitelist {
		if ip == wip {
			return true
		}
	}
	return false
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("/etc/ossh/")
		viper.AddConfigPath("$HOME/.ossh")
		viper.AddConfigPath(".")
	}

	err := viper.ReadInConfig()
	if err != nil {
		log.Panic(fmt.Errorf("[Config] Fatal error config file: %w", err))
	}
	cfgFile = viper.ConfigFileUsed()

	err = viper.Unmarshal(&Conf)
	if err != nil {
		log.Printf("[Config] Unable to decode into Config struct, %v", err)
	}

	if Conf.PathData == "" {
		Conf.PathData = "/etc/ossh"
	}

	if Conf.PathCaptures == "" {
		Conf.PathCaptures = fmt.Sprintf("%s/captures", Conf.PathData)
	}

	if Conf.PathCommands == "" {
		Conf.PathCommands = fmt.Sprintf("%s/commands", Conf.PathData)
	}

	if Conf.PathWebinterface == "" {
		Conf.PathWebinterface = fmt.Sprintf("%s/webinterface", Conf.PathData)
	}

	if Conf.PathFFS == "" {
		Conf.PathFFS = fmt.Sprintf("%s/ffs", Conf.PathData)
	}

	if Conf.PathFingerprints == "" {
		Conf.PathFingerprints = fmt.Sprintf("%s/fingerprints.txt", Conf.PathData)
	}

	if Conf.PathHosts == "" {
		Conf.PathHosts = fmt.Sprintf("%s/hosts.txt", Conf.PathData)
	}

	if Conf.PathPasswords == "" {
		Conf.PathPasswords = fmt.Sprintf("%s/passwords.txt", Conf.PathData)
	}

	if Conf.PathUsers == "" {
		Conf.PathUsers = fmt.Sprintf("%s/users.txt", Conf.PathData)
	}

	InitTemplaterFunctions()
	InitTemplaterFunctionsHTML()

	LogOK("Config file loaded: %s\n", colorWrap(cfgFile, colorOrange))
}

func getConfig() string {
	cfg, err := os.ReadFile(cfgFile)
	if err != nil {
		LogError(
			"Could not read config from '%s': %s\n",
			colorWrap(cfgFile, colorCyan),
			colorWrap(err.Error(), colorOrange),
		)
	}
	return string(cfg)
}

func updateConfig(config []byte) error {
	pathSrc := viper.ConfigFileUsed()
	pathBak := fmt.Sprintf("%s.bak", pathSrc)
	err := CopyFile(pathSrc, pathBak)
	if err != nil {
		LogError("Failed to backup config from %s to %s!\n", pathSrc, pathBak)
		return err
	}
	err = os.WriteFile(pathSrc, config, 0644)
	if err != nil {
		LogError("Failed to backup config from %s to %s!\n", pathSrc, pathBak)
		return err
	}
	LogSuccess("Written new config to: %s\n", pathSrc)

	initConfig()
	return nil
}
