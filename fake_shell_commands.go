package main

import (
	"fmt"
	"io"
	fso "io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/toxyl/glog"
	"github.com/toxyl/gutils"
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
	"scp":   cmdScp,
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
	fs.RecordWriteLn(gutils.GeneratePseudoEmptyString(0))

	return
}

func cmdRm(fs *FakeShell, line string) (exit bool) {
	parts := strings.Split(line, " ")

	if len(parts) < 2 {
		fs.RecordWriteLn("rm: missing operand")
		fs.RecordWriteLn("Try 'rm --help' for more information.")
		return
	}

	// TODO handle options
	parts = gutils.RemoveCommandFlags(parts)

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
	fs.RecordWriteLn(gutils.GeneratePseudoEmptyString(0))

	return
}

func cmdLs(fs *FakeShell, line string) (exit bool) {
	parts := strings.Split(line, " ")
	// TODO handle options
	parts = gutils.RemoveCommandFlags(parts)

	var dir string
	if len(parts) < 2 {
		dir = fs.cwd
	} else {
		dir = toAbs(fs, parts[1])
	}

	entries, err := fs.overlayFS.ReadDir(dir)
	if err != nil {
		if err.(*os.PathError).Err.Error() != "not a directory" {
			fs.RecordWriteLn(fmt.Sprintf("ls: cannot access '%s': %s", dir, gutils.GetLastError(err.(*os.PathError).Err)))
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
	parts = gutils.RemoveCommandFlags(parts)

	path := toAbs(fs, parts[1])
	file, err := fs.overlayFS.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		fs.RecordWriteLn(fmt.Sprintf("cat: %s: %s", parts[1], gutils.GetLastError(err)))
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		fs.RecordWriteLn(fmt.Sprintf("cat: %s: %s", parts[1], gutils.GetLastError(err)))
		return
	}

	if stat.IsDir() {
		fs.RecordWriteLn(fmt.Sprintf("cat: %s: Is a directory", parts[1]))
		return
	}

	fileContents, err := io.ReadAll(file)
	if err != nil {
		fs.RecordWriteLn(fmt.Sprintf("cat: %s: %s", parts[1], gutils.GetLastError(err)))
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
	parts = gutils.RemoveCommandFlags(parts)

	path := toAbs(fs, parts[1])
	file, err := fs.overlayFS.OpenFile(path, os.O_CREATE, 0)
	if err != nil {
		fs.RecordWriteLn(fmt.Sprintf("touch: %s: %s", parts[1], gutils.GetLastError(err)))
		return
	}
	defer file.Close()

	return
}

func cmdScp(fs *FakeShell, line string) (exit bool) {
	parts := strings.Split(line, " ")
	if len(parts) < 2 {
		fs.RecordWriteLn("usage: scp [-346ABCpqrTv] [-c cipher] [-F ssh_config] [-i identity_file]")
		fs.RecordWriteLn("[-J destination] [-l limit] [-o ssh_option] [-P port]")
		fs.RecordWriteLn("[-S program] source ... target")
		return
	}

	isSink := false
	for i, p := range parts {
		if i > 0 && strings.HasPrefix(p, "-") && strings.Contains(p, "t") {
			isSink = true
			break
		}
	}

	if isSink {
		// someone wants to donate a file
		fs.WriteBinary(0b0) // ready to receive
		dname := strings.Trim(parts[len(parts)-1], "\"'")
		dirs := []string{dname}

		// read all messages
		for {
			msgType, err := fs.ReadBytes(1)

			if err != nil {
				if err.Error() != "EOF" {
					fs.logger.Error("Could not read type: %s", glog.Error(err))
				}
				break
			}

			mt := msgType[0]

			if mt == 'C' {
				// C = single file copy
				msgMode, err := fs.ReadBytesUntil(' ')
				if err != nil {
					fs.logger.Error("Could not read mode: %s", glog.Error(err))
					break
				}
				msgLength, err := fs.ReadBytesUntil(' ')
				if err != nil {
					fs.logger.Error("Could not read length: %s", glog.Error(err))
					break
				}
				msgLengthInt := gutils.BytesToInt(msgLength, 0)

				msgFileName, err := fs.ReadBytesUntil('\n')
				if err != nil {
					fs.logger.Error("Could not read file name: %s", glog.Error(err))
					break
				}
				msgFileNameStr := string(msgFileName)
				msgFileNameStr = strings.Trim(msgFileNameStr, "'\"")
				msgFileNameFull := fmt.Sprintf("%s/%s", strings.Join(dirs, "/"), msgFileNameStr)

				fs.WriteBinary(0b0) // ready to receive

				path := toAbs(fs, msgFileNameFull)
				if fs.overlayFS == nil {
					fs.logger.Error("scp: %s: %s", msgFileNameStr, glog.Reason("no OverlayFS available!"))
					return
				}

				file, err := fs.overlayFS.OpenFile(path, os.O_RDWR|os.O_CREATE, fso.FileMode(gutils.BytesToInt(msgMode, 0777)))
				if err != nil && gutils.GetLastError(err) != "is a directory" {
					fs.logger.Error("scp: %s: %s", msgFileNameStr, gutils.GetLastError(err))
					return
				}
				defer file.Close()

				msgFileData, err := fs.ReadBytesUntilEOF(msgLengthInt)
				if err != nil && err.Error() != "EOF" {
					fs.logger.Error("Could not read file data: %s", glog.Error(err))
					break
				}

				_, _ = file.Write(msgFileData)

				fs.logger.OK("File uploaded via SCP: %s", glog.File(msgFileNameFull))
				fpath := filepath.Clean(fmt.Sprintf("%s/scp-uploads/%s", Conf.PathCaptures, msgFileNameFull))
				if !gutils.FileExists(fpath) {
					basedir := filepath.Dir(fpath)
					_ = os.MkdirAll(basedir, 0644)
					_ = os.WriteFile(fpath, msgFileData, 0400)
					fs.logger.OK("SCP upload saved to: %s", glog.File(fpath))
				}

				fs.WriteBinary(0b0) // data read
				continue
			}

			if mt == 'D' {
				// D = recursive dir copy
				msgMode, err := fs.ReadBytesUntil(' ')
				if err != nil {
					fs.logger.Error("Could not read mode: %s", glog.Error(err))
					break
				}

				_, err = fs.ReadBytesUntil(' ')
				if err != nil {
					fs.logger.Error("Could not read length: %s", glog.Error(err))
					break
				}

				msgDirName, err := fs.ReadBytesUntil('\n')
				if err != nil {
					fs.logger.Error("Could not read dir name: %s", glog.Error(err))
					break
				}

				msgDirNameStr := string(msgDirName)
				msgDirNameStr = strings.Trim(msgDirNameStr, "'\"")

				if fs.overlayFS == nil {
					fs.logger.Error("scp: %s: %s", msgDirNameStr, glog.Reason("no OverlayFS available!"))
					return
				}

				dirs = append(dirs, msgDirNameStr)
				_ = fs.overlayFS.MkdirAll(strings.Join(dirs, "/"), fso.FileMode(gutils.BytesToInt(msgMode, 0777)))

				fs.WriteBinary(0b0) // data read
				continue
			}

			if mt == 'E' {
				// end of dir
				dirs = dirs[0 : len(dirs)-1]

				fs.WriteBinary(0b0) // data read
				continue
			}

			if mt == 'T' {
				// modification time
				_, _ = fs.ReadBytesUntil('\n')

				fs.WriteBinary(0b0) // data read
				continue
			}
		}
	}
	return false
}
