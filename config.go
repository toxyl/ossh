package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/spf13/viper"
	"github.com/toxyl/glog"
	"github.com/toxyl/gutils"
)

const (
	INTERVAL_UI_STATS_UPDATE   = 10 * time.Second
	INTERVAL_STATS_BROADCAST   = 60 * time.Second
	INTERVAL_OVERLAYFS_CLEANUP = 30 * time.Second
	INTERVAL_SESSIONS_CLEANUP  = 1 * time.Minute
	INTERVAL_SYNC_CLEANUP      = 25 * time.Second
	DELAY_OVERLAYFS_MKDIR      = 100 * time.Millisecond
	CLEANUP_SYNC_MIN_AGE       = 60 * time.Second
)

var (
	regexEnvVarPrefixes = regexp.MustCompile(`[A-Z_\-0-9]+=.*?\s+(.*)`)
)

//go:embed commands/*
var fsCommandTemplates embed.FS

//go:embed webinterface/*
var fsWebinterfaceTemplates embed.FS

type Config struct {
	Debug struct {
		FakeShell    bool `mapstructure:"fake_shell"`
		SyncCommands bool `mapstructure:"sync_commands"`
		SyncServer   bool `mapstructure:"sync_server"`
		SyncClient   bool `mapstructure:"sync_client"`
		OSSHServer   bool `mapstructure:"ossh_server"`
		Sessions     bool `mapstructure:"sessions"`
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
	Hostnames        []struct {
		Name string `mapstructure:"name"`
		IP   string `mapstructure:"ip"`
	} `mapstructure:"hostnames"`
	Host           string  `mapstructure:"host"`
	Port           uint    `mapstructure:"port"`
	MaxIdleTimeout uint    `mapstructure:"max_idle"`
	MaxSessionAge  uint    `mapstructure:"max_session_age"`
	InputDelay     uint    `mapstructure:"input_delay"`
	Ratelimit      float64 `mapstructure:"ratelimit"`
	Webinterface   struct {
		Enabled  bool   `mapstructure:"enabled"`
		Host     string `mapstructure:"host"`
		Port     uint   `mapstructure:"port"`
		CertFile string `mapstructure:"cert_file"`
		KeyFile  string `mapstructure:"key_file"`
	} `mapstructure:"webinterface"`
	MetricsServer struct {
		Host string `mapstructure:"host"`
		Port uint   `mapstructure:"port"`
	} `mapstructure:"metrics_server"`
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

var LogGlobal *glog.Logger = glog.NewLogger("Global", glog.Gray, false, false, false, logMessageHandler)

func logMessageHandler(msg string) {
	fmt.Print(msg)
	if SrvUI != nil {
		SrvUI.PushLog(msg)
	}
}

func isIPWhitelisted(ip string) bool {
	for _, wip := range Conf.IPWhitelist {
		if ip == wip {
			return true
		}
	}
	return false
}

func colorConnID(user, host string, port int) string {
	addr := glog.AddrHostPort(host, port, true)
	if user == "" {
		return addr
	}
	return fmt.Sprintf("%s > %s", addr, glog.Wrap(user, glog.Green))
}

func initPath(p, d string) string {
	if p == "" {
		p = fmt.Sprintf("%s/%s", Conf.PathData, d)
	}
	return p
}

func InitPaths() {
	Conf.PathData = initPath(Conf.PathData, "/etc/ossh")
	Conf.PathCaptures = initPath(Conf.PathCaptures, "captures")
	Conf.PathCommands = initPath(Conf.PathCommands, "commands")
	Conf.PathWebinterface = initPath(Conf.PathWebinterface, "webinterface")
	Conf.PathFFS = initPath(Conf.PathFFS, "ffs")
	Conf.PathPayloads = initPath(Conf.PathPayloads, "payloads.txt")
	Conf.PathHosts = initPath(Conf.PathHosts, "hosts.txt")
	Conf.PathPasswords = initPath(Conf.PathPasswords, "passwords.txt")
	Conf.PathUsers = initPath(Conf.PathUsers, "users.txt")
	Conf.Webinterface.CertFile = initPath(Conf.Webinterface.CertFile, "ossh.crt")
	Conf.Webinterface.KeyFile = initPath(Conf.Webinterface.KeyFile, "ossh.key")

	err := gutils.MkDirs(
		Conf.PathCommands,
		Conf.PathCaptures,
		fmt.Sprintf("%s/%s", Conf.PathCaptures, "payloads"),
		fmt.Sprintf("%s/%s", Conf.PathCaptures, "scp-uploads"),
		fmt.Sprintf("%s/%s", Conf.PathCaptures, "ssh-keys"),
		Conf.PathFFS,
		Conf.PathWebinterface,
	)
	if err != nil {
		panic(err)
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

	err = gutils.CopyEmbeddedFSToDisk(fsCommandTemplates, Conf.PathCommands, "commands")
	if err != nil {
		log.Panicf("[Config] Unable to copy command templates to disk, %v", err)
	}
	err = gutils.CopyEmbeddedFSToDisk(fsWebinterfaceTemplates, Conf.PathWebinterface, "webinterface")
	if err != nil {
		log.Panicf("[Config] Unable to copy webinterface templates to disk, %v", err)
	}

	InitTemplaterFunctions()
	InitTemplaterFunctionsHTML()

	LogGlobal.OK("Config loaded from %s", glog.Wrap(cfgFile, glog.Orange))
}

func getConfig() string {
	cfg, err := os.ReadFile(cfgFile)
	if err != nil {
		LogGlobal.Error(
			"Could not read config from '%s': %s",
			glog.File(cfgFile),
			glog.Error(err),
		)
	}
	return string(cfg)
}

func updateConfig(config []byte) error {
	pathSrc := viper.ConfigFileUsed()
	pathBak := fmt.Sprintf("%s.bak", pathSrc)
	err := gutils.CopyFile(pathSrc, pathBak)
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
	SrvSync.UpdateClients()
	return nil
}
