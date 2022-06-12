package main

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// OverlayFSManager manages multiple OverlayFS's. It maintains the following directory hierarchy:
// baseDir
// |- defaultfs
// |  |- /etc
// |  	 |- shadow
// |  |- ...ect
// |- sandboxes
// |  |- 123.12.1.2
// |  |  |- merged-1651413027
// |  |  |- work-1651413027
// |  |  |- merged-1651401234
// |  |  |- work-1651401234
// |  |  |- layers
// |  |     |- 1651413027
// |  |     |- 1651401234
// |  |- 127.0.0.1
// |     |- merged-1634115023
// |     |- work-1634115023
// |     |- layers
// |        |- 1634115023
// |        |- 1632105423
//
// defaultfs is always the lower layer for every sandbox it contains the default FS with which each FS sandbox starts.
// Each sandbox is identified by its sandbox key, which can anything(source IP's were chosen in the example). Each
// sandbox has a layers directory containing all layers which make up the merged layers. Each "session" gets its own
// merged-... directory which is where the OverlayFS will be mounted. A sandbox can have multiple active sessions
// however, each session always has a unique upper-dir.
type OverlayFSManager struct {
	baseDir string

	mu             sync.Mutex
	activeOverlays map[string]bool
}

//go:embed ffs
var defaultFS embed.FS

func (ofsm *OverlayFSManager) Init(baseDir string) error {
	LogOverlayFS.Debug("init %s", colorFile(baseDir))
	if !DirExists(baseDir) {
		err := os.Mkdir(baseDir, 0755)
		if err != nil {
			return fmt.Errorf("can't make baseDir: %w", err)
		}
	}

	defaultFsPath := filepath.Join(baseDir, "defaultfs")
	if !DirExists(defaultFsPath) {
		err := os.Mkdir(defaultFsPath, 0755)
		if err != nil {
			return fmt.Errorf("can't make defaultfs dir: %w", err)
		}

		// Copy embedded fs to disk
		err = fs.WalkDir(defaultFS, ".", func(path string, d fs.DirEntry, err error) error {
			if strings.HasPrefix(path, "ffs/") {
				subPath := strings.TrimPrefix(path, "ffs/")

				info, err := d.Info()
				if err != nil {
					return err
				}

				if d.IsDir() {
					// TODO correct dir permission in later pass
					err = os.Mkdir(filepath.Join(defaultFsPath, subPath), 0755)
					if err != nil {
						return err
					}

					return nil
				}

				data, err := defaultFS.ReadFile(path)
				if err != nil {
					return err
				}

				err = ioutil.WriteFile(filepath.Join(defaultFsPath, subPath), data, info.Mode())
				if err != nil {
					return err
				}
			}

			return nil
		})
		if err != nil {
			return fmt.Errorf("can't walk embedded dir: %w", err)
		}
	}

	if !DirExists(filepath.Join(baseDir, "sandboxes")) {
		err := os.Mkdir(filepath.Join(baseDir, "sandboxes"), 0755)
		if err != nil {
			return fmt.Errorf("can't make defaultfs dir: %w", err)
		}
	}

	ofsm.baseDir = baseDir
	ofsm.activeOverlays = make(map[string]bool)
	go ofsm.CleanupWorker()

	return nil
}

func (ofsm *OverlayFSManager) NewSession(sandboxKey string) (*OverlayFS, error) {
	sandboxPath := filepath.Join(ofsm.baseDir, "sandboxes", sandboxKey)
	LogOverlayFS.Debug("Creating new session for %s at %s", colorHighlight(sandboxKey), colorFile(sandboxPath))
	if !DirExists(sandboxPath) {
		err := os.Mkdir(sandboxPath, 0755)
		if err != nil {
			return nil, fmt.Errorf("make sandbox dir: %w", err)
		}
	}

	sandboxLayersPath := filepath.Join(sandboxPath, "layers")
	if !DirExists(sandboxLayersPath) {
		err := os.Mkdir(sandboxLayersPath, 0755)
		if err != nil {
			return nil, fmt.Errorf("make sandbox dir: %w", err)
		}
	}

	timeKey := strconv.FormatInt(time.Now().UnixNano(), 10)

	mergeLayerPath := filepath.Join(sandboxPath, fmt.Sprintf("merge-%s", timeKey))
	workLayerPath := filepath.Join(sandboxPath, fmt.Sprintf("work-%s", timeKey))
	upperLayerPath := filepath.Join(sandboxPath, "layers", timeKey)
	var lowerLayers []string

	entries, err := os.ReadDir(sandboxLayersPath)
	if err != nil {
		return nil, fmt.Errorf("read layers dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		lowerLayers = append(lowerLayers, filepath.Join(sandboxPath, "layers", entry.Name()))
	}

	sort.Slice(lowerLayers, func(i, j int) bool {
		numA, _ := strconv.Atoi(lowerLayers[i])
		numB, _ := strconv.Atoi(lowerLayers[j])
		return numA < numB
	})

	lowerLayers = append(lowerLayers, filepath.Join(ofsm.baseDir, "defaultfs"))

	ofsm.mu.Lock()
	ofsm.activeOverlays[mergeLayerPath] = true
	ofsm.mu.Unlock()

	return &OverlayFS{
		manager:   ofsm,
		mergedDir: mergeLayerPath,
		upperDir:  upperLayerPath,
		workDir:   workLayerPath,
		lowerDirs: lowerLayers,
	}, nil
}

func (ofsm *OverlayFSManager) CleanupWorker() {
	sandboxPath := filepath.Join(ofsm.baseDir, "sandboxes")

	for {
		time.Sleep(INTERVAL_OVERLAYFS_CLEANUP)

		sandboxes, err := os.ReadDir(sandboxPath)
		if err != nil {
			LogOverlayFS.Error("cleanup worker: %s", err.Error())
			continue
		}

		for _, sandbox := range sandboxes {
			sandboxEntries, err := os.ReadDir(filepath.Join(sandboxPath, sandbox.Name()))
			if err != nil {
				LogOverlayFS.Error("cleanup worker, read sandbox dir: %s", err.Error())
				continue
			}

			for _, entry := range sandboxEntries {
				if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "merge-") {
					continue
				}

				mergeDirPath := filepath.Join(sandboxPath, sandbox.Name(), entry.Name())
				ofsm.mu.Lock()
				active := ofsm.activeOverlays[mergeDirPath]
				ofsm.mu.Unlock()

				if !active {
					timestamp := strings.Split(entry.Name(), "-")[1]

					err = (&OverlayFS{
						mergedDir: mergeDirPath,
						workDir:   filepath.Join(sandboxPath, sandbox.Name(), fmt.Sprintf("work-%s", timestamp)),
					}).Unmount()

					if err != nil && !strings.HasPrefix(err.Error(), "unmount: invalid argument") { // seems that 'unmount: invalid argument' is safe to ignore
						LogOverlayFS.Error("cleanup worker, close overlay '%s': %s", mergeDirPath, colorError(err))
						continue
					}
				}
			}
		}
	}
}

func (ofsm *OverlayFSManager) DeactivateOverlay(fs *OverlayFS) {
	ofsm.mu.Lock()
	defer ofsm.mu.Unlock()
	delete(ofsm.activeOverlays, fs.mergedDir)
}

// https://windsock.io/the-overlay-filesystem/
type OverlayFS struct {
	manager *OverlayFSManager

	// The dir containing the merged layers
	mergedDir string
	// The upper most layer, containing all changed made if any
	upperDir string
	// The work dir
	workDir string
	// The lower layers, ordered by time
	lowerDirs []string
}

func (ofs *OverlayFS) Mount() error {
	LogOverlayFS.Debug("mount %s", colorFile(ofs.mergedDir))
	if !DirExists(ofs.mergedDir) {
		err := os.Mkdir(ofs.mergedDir, 700)
		if err != nil {
			return fmt.Errorf("mkdir merged (%s): %w", ofs.mergedDir, err)
		}
		time.Sleep(DELAY_OVERLAYFS_MKDIR)
	}

	if !DirExists(ofs.workDir) {
		err := os.Mkdir(ofs.workDir, 700)
		if err != nil {
			return fmt.Errorf("mkdir workdir (%s): %w", ofs.workDir, err)
		}
		time.Sleep(DELAY_OVERLAYFS_MKDIR)
	}

	if !DirExists(ofs.upperDir) {
		err := os.Mkdir(ofs.upperDir, 700)
		if err != nil {
			return fmt.Errorf("mkdir upper (%s): %w", ofs.upperDir, err)
		}
		time.Sleep(DELAY_OVERLAYFS_MKDIR)
	}

	lowerdirs := strings.Join(ofs.lowerDirs, ":")
	data := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerdirs, ofs.upperDir, ofs.workDir)

	if DirExists(ofs.mergedDir) {
		err := unix.Mount("overlay", ofs.mergedDir, "overlay", 0, data)
		if err != nil {
			return fmt.Errorf("mount (%s): %w", ofs.mergedDir, err)
		}
		return nil
	}

	return fmt.Errorf("mount (%s): %s", ofs.mergedDir, "the directory does not exist")
}

func (ofs *OverlayFS) Close() {
	ofs.manager.DeactivateOverlay(ofs)
}

func (ofs *OverlayFS) Unmount() error {
	LogOverlayFS.Debug("unmount %s", colorFile(ofs.mergedDir))
	err := unix.Unmount(ofs.mergedDir, syscall.MNT_DETACH)
	if err != nil {
		return fmt.Errorf("unmount: %w", err)
	}

	err = os.Remove(ofs.mergedDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove mergedDir: %w", err)
		}
	}

	err = os.RemoveAll(ofs.workDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove workdir: %w", err)
		}
	}

	return nil
}

func (ofs *OverlayFS) insideMerged(path string) bool {
	mergedAbs, err := filepath.Abs(ofs.mergedDir)
	if err != nil {
		panic(err)
	}

	absPath, err := filepath.Abs(filepath.Join(mergedAbs, path))
	if err != nil {
		return false
	}

	return strings.HasPrefix(absPath, mergedAbs)
}

func (ofs *OverlayFS) RemoveFile(path string, recursive bool) error {
	LogOverlayFS.Info("Remove %s%s", colorFile(ofs.mergedDir), colorReason(path))

	if !ofs.insideMerged(path) {
		return errors.New("path outside root")
	}
	if recursive {
		return os.RemoveAll(filepath.Join(ofs.mergedDir, path))
	}
	return os.Remove(filepath.Join(ofs.mergedDir, path))
}

func (ofs *OverlayFS) OpenFile(path string, flag int, perm fs.FileMode) (*os.File, error) {
	LogOverlayFS.Info("Open %s%s", colorFile(ofs.mergedDir), colorReason(path))

	if !ofs.insideMerged(path) {
		return nil, errors.New("path outside root")
	}

	// create the directory structure, so we don't get not found errors
	// because our fake file system is incomplete
	_ = ofs.MkdirAll(filepath.Dir(path), perm)

	return os.OpenFile(filepath.Join(ofs.mergedDir, path), flag, perm)
}

func (ofs *OverlayFS) DirExists(path string) bool {
	if !ofs.insideMerged(path) {
		return false
	}

	return DirExists(filepath.Join(ofs.mergedDir, path))
}

func (ofs *OverlayFS) FileExists(path string) bool {
	if !ofs.insideMerged(path) {
		return false
	}

	return FileExists(filepath.Join(ofs.mergedDir, path))
}

func (ofs *OverlayFS) Mkdir(path string, mode fs.FileMode) error {
	LogOverlayFS.Debug("mkdir %s", colorFile(path))
	if !ofs.insideMerged(path) {
		return errors.New("path outside root")
	}

	return os.Mkdir(filepath.Join(ofs.mergedDir, path), mode)
}

func (ofs *OverlayFS) MkdirAll(path string, mode fs.FileMode) error {
	LogOverlayFS.Debug("mkdir-all %s", colorFile(path))
	if !ofs.insideMerged(path) {
		return errors.New("path outside root")
	}

	return os.MkdirAll(filepath.Join(ofs.mergedDir, path), mode)
}

func (ofs *OverlayFS) ReadDir(path string) ([]os.DirEntry, error) {
	LogOverlayFS.Debug("readdir %s", colorFile(path))
	if !ofs.insideMerged(path) {
		return nil, errors.New("path outside root")
	}

	return os.ReadDir(filepath.Join(ofs.mergedDir, path))
}
