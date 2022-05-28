package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Command func(fs *FakeShell, line string) (exit bool)

var CmdLookup = map[string]Command{
	"cd":    cmdCd,
	"ls":    cmdLs,
	"dir":   cmdLs, // TODO make a separate dir command?
	"pwd":   cmdPwd,
	"cat":   cmdCat,
	"touch": cmdTouch,
	"rm":    cmdRm,
}

func toAbs(fs *FakeShell, path string) string {
	if !strings.HasPrefix(path, "/") {
		path = filepath.Clean(filepath.Join(fs.cwd, path))
	}

	return path
}

func cmdCd(fs *FakeShell, line string) (exit bool) {
	parts := strings.Split(line, " ")
	var path string

	if len(parts) == 1 {
		parts = append(parts, "~")
	}

	if len(parts) < 2 {
		path = filepath.Join("/home", fs.User())
	} else {
		path = parts[1]
	}

	if strings.HasPrefix(parts[1], "~") {
		path = filepath.Join("/home", fs.User(), strings.TrimPrefix(path, "~"))
	}

	path = toAbs(fs, path)

	if !fs.overlayFS.DirExists(path) {
		fs.RecordWriteLn(fmt.Sprintf("cd: %s: no such file or directory", parts[1]))
		return
	}

	fs.cwd = path

	if path == filepath.Join("/home", fs.User()) {
		fs.UpdatePrompt("~")
	} else {
		fs.UpdatePrompt(filepath.Base(path))
	}

	// cd runs way too fast without any output,
	// let's fuck a bit with the bots
	fs.RecordWriteLn(GeneratePseudoEmptyString(0))

	return
}

func cmdRm(fs *FakeShell, line string) (exit bool) {
	parts := strings.Split(line, " ")

	if len(parts) == 1 {
		fs.RecordWriteLn("rm: missing operand")
		fs.RecordWriteLn("Try 'rm --help' for more information.")
		return
	}

	// TODO handle options

	for _, pt := range parts[1:] {
		if strings.HasPrefix(pt, "~") {
			pt = filepath.Join("/home", fs.User(), strings.TrimPrefix(pt, "~"))
		}
		path := toAbs(fs, pt)

		if !fs.overlayFS.DirExists(path) && !fs.overlayFS.FileExists(path) {
			fs.RecordWriteLn(fmt.Sprintf("rm: %s: no such file or directory", pt))
			return
		}

		_ = fs.overlayFS.RemoveFile(path, false)
	}

	// rm runs way too fast without any output,
	// let's fuck a bit with the bots
	fs.RecordWriteLn(GeneratePseudoEmptyString(0))

	return
}

func cmdLs(fs *FakeShell, line string) (exit bool) {
	parts := strings.Split(line, " ")
	var dir string
	if len(parts) < 2 {
		dir = fs.cwd
	} else {
		dir = toAbs(fs, parts[1])
	}

	// TODO handle options

	entries, err := fs.overlayFS.ReadDir(dir)
	if err != nil {
		if err.(*os.PathError).Err.Error() != "not a directory" {
			fs.RecordWriteLn(fmt.Sprintf("ls: cannot access '%s': %s", dir, GetLastError(err.(*os.PathError).Err)))
			return
		}

		// "not a directory" means the path is a file, list it
		fs.RecordWriteLn(parts[1])
		return
	}

	// TODO check term width and fit grid like the real ls

	for _, entry := range entries {
		fs.RecordWrite(entry.Name())
		fs.RecordWrite(" ")
	}
	fs.RecordWrite("\n")

	return
}

func cmdPwd(fs *FakeShell, line string) (exit bool) {
	fs.RecordWriteLn(fs.cwd)
	return
}

func cmdCat(fs *FakeShell, line string) (exit bool) {
	parts := strings.Split(line, " ")
	if len(parts) < 2 {
		// TODO echo input, like the real `cat` command
		fs.RecordWriteLn("cat: specify file")
		return
	}

	// TODO handle flags

	path := toAbs(fs, parts[1])
	file, err := fs.overlayFS.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		fs.RecordWriteLn(fmt.Sprintf("cat: %s: %s", parts[1], GetLastError(err)))
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		fs.RecordWriteLn(fmt.Sprintf("cat: %s: %s", parts[1], GetLastError(err)))
		return
	}

	if stat.IsDir() {
		fs.RecordWriteLn(fmt.Sprintf("cat: %s: Is a directory", parts[1]))
		return
	}

	fileContents, err := io.ReadAll(file)
	if err != nil {
		fs.RecordWriteLn(fmt.Sprintf("cat: %s: %s", parts[1], GetLastError(err)))
		return
	}

	fs.RecordWrite(string(fileContents))

	return
}

func cmdTouch(fs *FakeShell, line string) (exit bool) {
	parts := strings.Split(line, " ")
	if len(parts) < 2 {
		fs.RecordWriteLn("touch: specify file")
		return
	}

	// TODO handle flags

	path := toAbs(fs, parts[1])
	file, err := fs.overlayFS.OpenFile(path, os.O_CREATE, 0)
	if err != nil {
		fs.RecordWriteLn(fmt.Sprintf("touch: %s: %s", parts[1], GetLastError(err)))
		return
	}
	defer file.Close()

	return
}
