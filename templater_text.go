package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/davecgh/go-spew/spew"
)

var templateFunctions template.FuncMap = template.FuncMap{}

func parseTemplateDir(dir string) (*template.Template, error) {
	var paths []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return template.New(dir).Funcs(templateFunctions).ParseFiles(paths...)
}

func parseTemplateString(templateString string, wr io.Writer, data interface{}) error {
	t, err := template.New("tpl").Funcs(templateFunctions).Parse(templateString)
	if err != nil {
		if strings.Contains(err.Error(), "no template") {
			LogTextTemplater.Error("Template '%s' not found", templateString)
		} else {
			LogTextTemplater.Error("Failed to parse template string %s: %s", templateString, err.Error())
		}
		return err
	}
	return t.Execute(wr, data)
}

func ParseTemplateFromString(templateString string, data interface{}) string {
	var tpl bytes.Buffer
	err := parseTemplateString(templateString, &tpl, data)
	if err != nil {
		return ""
	}
	return strings.Trim(tpl.String(), " \r\n")
}

func ParseTemplate(name string, wr io.Writer, data interface{}) error {
	dir := Conf.PathCommands
	_, err := os.Stat(dir)
	if err != nil {
		return err
	}

	t, err := parseTemplateDir(dir)

	if err != nil {
		return err
	}

	return t.ExecuteTemplate(wr, name, data)
}

func ParseTemplateToString(name string, data interface{}) string {
	var tpl bytes.Buffer
	err := ParseTemplate(name, &tpl, data)
	if err != nil {
		if strings.Contains(err.Error(), "no template") {
			LogTextTemplater.Error("Template '%s' not found", name)
		} else {
			LogTextTemplater.Error("Failed to parse template string %s: %s", name, err.Error())
		}
		return fmt.Sprintf("%s: command not found", name)
	}
	return strings.Trim(tpl.String(), " \r\n")
}

func InitTemplaterFunctions() {
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
