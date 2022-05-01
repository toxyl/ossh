package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/davecgh/go-spew/spew"
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
	Sync             struct {
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
var FFS FakeFileSystem

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

	FFS = FakeFileSystem{
		Root: Conf.PathFFS,
		CWD:  "~",
		User: "root",
	}

	templateFunctions = template.FuncMap{
		"nl": func() string {
			return "\n"
		},
		"subint": func(a, b int) int {
			return a - b
		},
		"sub": func(a, b float64) float64 {
			return a - b
		},
		"add": func(a, b interface{}) float64 {
			af, _ := GetFloat(a)
			bf, _ := GetFloat(b)
			return af + bf
		},
		"div": func(a, b float64) float64 {
			return a / b
		},
		"mul": func(a, b float64) float64 {
			return a * b
		},
		"file": func(path string) string {
			return FFS.Read(path)
		},
		"list": func(path string) string {
			files := FFS.List(path)
			return strings.Join(files, " ")
		},
		"sha1": func(s string) string {
			return StringToSha1(s)
		},
		"sha256": func(s string) string {
			return StringToSha256(s)
		},
		"replace": func(s, re, repl string) string {
			rx := regexp.MustCompile(re)
			s = rx.ReplaceAllString(s, repl)
			return s
		},
		"lower": func(s string) string {
			return strings.ToLower(s)
		},
		"upper": func(s string) string {
			return strings.ToUpper(s)
		},
		"trim": func(s string) string {
			return strings.Trim(s, " \r\n")
		},
		"time": func(prefix, suffix string) string {
			return fmt.Sprintf("%s%d%s", prefix, time.Now().Unix(), suffix)
		},
		"concat": func(a, b string) string {
			return fmt.Sprintf("%s%s", a, b)
		},
		"dict": func(values ...interface{}) (map[string]interface{}, error) {
			if len(values)%2 != 0 {
				return nil, errors.New("invalid dict call")
			}
			dict := make(map[string]interface{}, len(values)/2)
			for i := 0; i < len(values); i += 2 {
				key, ok := values[i].(string)
				if !ok {
					return nil, errors.New("dict keys must be strings")
				}
				dict[key] = values[i+1]
			}
			return dict, nil
		},
		"dump": func(v interface{}) string {
			spew.Dump(v)
			return ""
		},
		"template_string": func(name string, values interface{}) (string, error) {
			var tpl bytes.Buffer
			_ = ParseTemplate(name, &tpl, values)
			return strings.ReplaceAll(strings.Trim(tpl.String(), " \r\n\t"), "\n", ""), nil
		},
	}

}
