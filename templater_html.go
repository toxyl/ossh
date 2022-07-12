package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	templatehtml "html/template"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/toxyl/gutils"
)

var templateFunctionsHTML template.FuncMap = template.FuncMap{}

func parseTemplateDirHTML(dir string) (*template.Template, error) {
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
	return template.New(dir).Funcs(templateFunctionsHTML).ParseFiles(paths...)
}

func ParseTemplateHTML(name string, wr io.Writer, data interface{}) error {
	dir := Conf.PathWebinterface
	_, err := os.Stat(dir)
	if err != nil {
		return err
	}

	t, err := parseTemplateDirHTML(dir)

	if err != nil {
		return err
	}

	return t.ExecuteTemplate(wr, name, data)
}

func InitTemplaterFunctionsHTML() {
	templateFunctionsHTML = templatehtml.FuncMap{
		"sub": func(a, b float64) float64 {
			return a - b
		},
		"add": func(a, b interface{}) float64 {
			af, _ := gutils.GetFloat(a)
			bf, _ := gutils.GetFloat(b)
			return af + bf
		},
		"div": func(a, b float64) float64 {
			return a / b
		},
		"mul": func(a, b float64) float64 {
			return a * b
		},
		"nbsp": func(s string) templatehtml.HTML {
			return templatehtml.HTML(strings.Replace(s, " ", "&nbsp;", -1))
		},
		"sha256": func(s string) string {
			return gutils.StringToSha256(s)
		},
		"replace": func(s, re, repl string) string {
			rx := regexp.MustCompile(re)
			s = rx.ReplaceAllString(s, repl)
			return s
		},
		"raw": func(s string) templatehtml.HTML {
			return templatehtml.HTML(s)
		},
		"rawjs": func(s string) templatehtml.JS {
			return templatehtml.JS(s)
		},
		"rawjs2str": func(s templatehtml.JS) string {
			return string(s)
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
