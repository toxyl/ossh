package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/viper"
)

const (
	INTERVAL_UI_STATS_UPATE    = 5 * time.Second
	INTERVAL_STATS_BROADCAST   = 20 * time.Second
	INTERVAL_OVERLAYFS_CLEANUP = 30 * time.Second
	INTERVAL_SESSIONS_CLEANUP  = 1 * time.Minute
	DELAY_OVERLAYFS_MKDIR      = 100 * time.Millisecond
	DELAY_SYNC_START           = 10 * time.Second
)

type Config struct {
	Debug struct {
		ASCIICastV2  bool `mapstructure:"asciicast_v2"`
		FakeShell    bool `mapstructure:"fake_shell"`
		SyncCommands bool `mapstructure:"sync_commands"`
		SyncServer   bool `mapstructure:"sync_server"`
		SyncClient   bool `mapstructure:"sync_client"`
		OSSHServer   bool `mapstructure:"ossh_server"`
		UIServer     bool `mapstructure:"ui_server"`
		OverlayFS    bool `mapstructure:"overlay_fs"`
	} `mapstructure:"debug"`
	PathData         string   `mapstructure:"path_data"`
	PathPayloads     string   `mapstructure:"path_payloads"`
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
	MaxSessionAge    uint     `mapstructure:"max_session_age"`
	InputDelay       uint     `mapstructure:"input_delay"`
	Ratelimit        float64  `mapstructure:"ratelimit"`
	Webinterface     struct {
		Host     string `mapstructure:"host"`
		Port     uint   `mapstructure:"port"`
		CertFile string `mapstructure:"cert_file"`
		KeyFile  string `mapstructure:"key_file"`
	} `mapstructure:"webinterface"`
	SyncServer struct {
		Host string `mapstructure:"host"`
		Port uint   `mapstructure:"port"`
	} `mapstructure:"sync_server"`
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
		Bullshit         []string   `mapstructure:"bullshit"`
	} `mapstructure:"commands"`
}

var cfgFile string = ""
var Conf Config

func isIPWhitelisted(ip string) bool {
	for _, wip := range Conf.IPWhitelist {
		if ip == wip {
			return true
		}
	}
	return false
}

func InitPaths() {
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

	if Conf.PathPayloads == "" {
		Conf.PathPayloads = fmt.Sprintf("%s/payloads.txt", Conf.PathData)
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

	if Conf.Webinterface.CertFile == "" {
		Conf.Webinterface.CertFile = fmt.Sprintf("%s/ossh.crt", Conf.PathData)
	}

	if Conf.Webinterface.KeyFile == "" {
		Conf.Webinterface.KeyFile = fmt.Sprintf("%s/ossh.key", Conf.PathData)
	}
}

func InitDebug() {
	if Conf.Debug.ASCIICastV2 {
		LogASCIICastV2.EnableDebug()
	}

	if Conf.Debug.SyncCommands {
		LogSyncCommands.EnableDebug()
	}

	if Conf.Debug.SyncClient {
		LogSyncClient.EnableDebug()
	}

	if Conf.Debug.SyncServer {
		LogSyncServer.EnableDebug()
	}

	if Conf.Debug.OSSHServer {
		LogOSSHServer.EnableDebug()
	}

	if Conf.Debug.UIServer {
		LogUIServer.EnableDebug()
	}

	if Conf.Debug.OverlayFS {
		LogOverlayFS.EnableDebug()
	}

	if Conf.Debug.FakeShell {
		LogFakeShell.EnableDebug()
	}
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
		log.Panicf("[Config] Unable to decode into Config struct, %v", err)
	}

	InitPaths()
	InitDebug()
	InitTemplaterFunctions()
	InitTemplaterFunctionsHTML()

	LogGlobal.OK("Config loaded from %s", colorWrap(cfgFile, colorOrange))
}

func getConfig() string {
	cfg, err := os.ReadFile(cfgFile)
	if err != nil {
		LogGlobal.Error(
			"Could not read config from '%s': %s",
			colorFile(cfgFile),
			colorError(err),
		)
	}
	return string(cfg)
}

func updateConfig(config []byte) error {
	pathSrc := viper.ConfigFileUsed()
	pathBak := fmt.Sprintf("%s.bak", pathSrc)
	err := CopyFile(pathSrc, pathBak)
	if err != nil {
		LogGlobal.Error("Failed to backup config from %s to %s!", pathSrc, pathBak)
		return err
	}
	err = os.WriteFile(pathSrc, config, 0644)
	if err != nil {
		LogGlobal.Error("Failed to backup config from %s to %s!", pathSrc, pathBak)
		return err
	}
	LogGlobal.Success("Written new config to: %s", pathSrc)

	initConfig()
	return nil
}
