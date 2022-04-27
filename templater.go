package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
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
			Log('x', "Template '%s' not found\n", templateString)
		} else {
			Log('x', "Failed to parse template string %s: %s\n", templateString, err.Error())
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
			Log('x', "Template '%s' not found\n", name)
		} else {
			Log('x', "Failed to parse template string %s: %s\n", name, err.Error())
		}
		return fmt.Sprintf("%s: command not found", name)
	}
	return strings.Trim(tpl.String(), " \r\n")
}
