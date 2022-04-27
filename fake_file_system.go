package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type FakeFileSystem struct {
	Root string
	CWD  string
	User string
}

func (ffs *FakeFileSystem) sanitize(file string) (string, error) {
	if strings.ContainsRune(file, '~') {
		if ffs.User == "root" {
			file = strings.ReplaceAll(file, "~", "/root")
		} else {
			file = strings.ReplaceAll(file, "~", fmt.Sprintf("/home/%s", ffs.User))
		}
	}

	f := fmt.Sprintf("%s/%s", ffs.Root, file)
	path, err := filepath.Rel(ffs.Root, f)
	if err != nil {
		Log('x', "Failed to resolve file '%s'.\nError: %s\n", file, err.Error())
		return "", err
	}
	if path == "" || path[0:3] == "../" {
		Log('!', "Something is fishy here, who wants '%s'?\n", path)
		return "", fmt.Errorf("%s: No such file or directory", file)
	}
	return f, nil
}

func (ffs *FakeFileSystem) IsDir(path string) bool {
	file, err := ffs.sanitize(path)
	if err != nil {
		return false
	}
	return DirExists(file)
}

func (ffs *FakeFileSystem) ChangeDir(path string) bool {
	dir, err := ffs.sanitize(path)
	if err != nil {
		return false
	}
	if DirExists(dir) {
		ffs.CWD = path
		return true
	}
	return false
}

func (ffs *FakeFileSystem) Exists(path string) bool {
	file, err := ffs.sanitize(path)
	if err != nil {
		return false
	}
	return DirExists(file) || FileExists(file)
}

func (ffs *FakeFileSystem) Read(path string) string {
	file, err := ffs.sanitize(path)
	if err != nil {
		return fmt.Errorf("%s: No such file or directory", path).Error()
	}
	if FileExists(file) {
		content, err := os.ReadFile(file)
		if err != nil {
			Log('x', "Failed to read file '%s'.\nError: %s\n", path, err.Error())
			return ""
		}
		return string(content)
	}
	return fmt.Errorf("%s: No such file or directory", path).Error()
}

func (ffs *FakeFileSystem) List(directory string) []string {
	if directory[:len(directory)-1] != "/" {
		directory += "/"
	}
	files := []string{}
	fs, err := ioutil.ReadDir(fmt.Sprintf("%s/%s", ffs.Root, directory))
	if err == nil {
		for _, file := range fs {
			files = append(files, fmt.Sprintf("%s%s", directory, file.Name()))
		}
	}
	for i, f := range files {
		files[i] = strings.ReplaceAll(f, directory, "")
	}
	return files
}
