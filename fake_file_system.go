package main

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/toxyl/glog"
	"github.com/toxyl/gutils"
	"golang.org/x/sys/unix"
)

// FakeFSManager manages multiple FakeFS's. It maintains the following directory hierarchy:
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
// merged-... directory which is where the FakeFS will be mounted. A sandbox can have multiple active sessions
// however, each session always has a unique upper-dir.
type FakeFSManager struct {
	baseDir        string
	mu             sync.Mutex
	activeOverlays map[string]bool
	overlays       map[string]*FakeFS
	logger         *glog.Logger
}

//go:embed ffs
var embeddedFS embed.FS

func (ofsm *FakeFSManager) Init(baseDir string) error {
	ofsm.logger = glog.NewLogger("Fake FS", glog.LightBlue, Conf.Debug.OverlayFS, logMessageHandler)
	ofsm.logger.Debug("Init %s", glog.File(baseDir))
	if !gutils.DirExists(baseDir) {
		err := os.Mkdir(baseDir, 0755)
		if err != nil {
			return fmt.Errorf("can't make baseDir: %w", err)
		}
	}

	defaultFsPath := filepath.Join(baseDir, "defaultfs")
	if !gutils.DirExists(defaultFsPath) {
		err := os.Mkdir(defaultFsPath, 0755)
		if err != nil {
			return fmt.Errorf("can't make defaultfs dir: %w", err)
		}

		// Copy embedded fs to disk
		err = fs.WalkDir(embeddedFS, ".", func(path string, d fs.DirEntry, err error) error {
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

				data, err := embeddedFS.ReadFile(path)
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

	if !gutils.DirExists(filepath.Join(baseDir, "sandboxes")) {
		err := os.Mkdir(filepath.Join(baseDir, "sandboxes"), 0755)
		if err != nil {
			return fmt.Errorf("can't make defaultfs dir: %w", err)
		}
	}

	ofsm.baseDir = baseDir
	ofsm.activeOverlays = make(map[string]bool)
	ofsm.overlays = make(map[string]*FakeFS)

	return nil
}

func (ofsm *FakeFSManager) NewSession(sandboxKey string) (*FakeFS, error) {
	sandboxPath := filepath.Join(ofsm.baseDir, "sandboxes", sandboxKey)

	if !gutils.DirExists(sandboxPath) {
		err := os.MkdirAll(sandboxPath, 0755)
		if err != nil {
			return nil, fmt.Errorf("make sandbox dir: %w", err)
		}
	}

	sandboxLayersPath := filepath.Join(sandboxPath, "layers")
	if !gutils.DirExists(sandboxLayersPath) {
		err := os.MkdirAll(sandboxLayersPath, 0755)
		if err != nil {
			return nil, fmt.Errorf("make sandbox dir: %w", err)
		}
	}

	mergeLayerPath := filepath.Join(sandboxPath, "merge-data")
	workLayerPath := filepath.Join(sandboxPath, "work-data")
	upperLayerPath := filepath.Join(sandboxPath, "layers", "data")
	var lowerLayers []string

	ofsm.mu.Lock()
	if _, ok := ofsm.activeOverlays[mergeLayerPath]; ok {
		if ofsm.activeOverlays[mergeLayerPath] {
			if v, ok := ofsm.overlays[mergeLayerPath]; ok {
				ofsm.mu.Unlock()
				ofsm.logger.Debug("Returning existing session for %s at %s", glog.Highlight(sandboxKey), glog.File(sandboxPath))
				return v, nil
			}
		}
	}
	ofsm.mu.Unlock()

	ofsm.logger.Debug("Creating new session for %s at %s", glog.Highlight(sandboxKey), glog.File(sandboxPath))

	lowerLayers = append(lowerLayers, filepath.Join(ofsm.baseDir, "defaultfs"))

	ofs := &FakeFS{
		manager:   ofsm,
		mergedDir: mergeLayerPath,
		upperDir:  upperLayerPath,
		workDir:   workLayerPath,
		lowerDirs: lowerLayers,
		logger:    ofsm.logger,
	}

	ofsm.mu.Lock()
	ofsm.activeOverlays[mergeLayerPath] = true
	ofsm.overlays[mergeLayerPath] = ofs
	ofsm.mu.Unlock()

	return ofs, nil
}

func (ofsm *FakeFSManager) DeactivateOverlay(fs *FakeFS) {
	ofsm.mu.Lock()
	defer ofsm.mu.Unlock()
	delete(ofsm.activeOverlays, fs.mergedDir)
	delete(ofsm.overlays, fs.mergedDir)
}

// https://windsock.io/the-overlay-filesystem/
type FakeFS struct {
	manager   *FakeFSManager
	logger    *glog.Logger
	mergedDir string   // The dir containing the merged layers
	upperDir  string   // The upper most layer, containing all changed made if any
	workDir   string   // The work dir
	lowerDirs []string // The lower layers, ordered by time
}

func (ofs *FakeFS) Mount() error {
	ofs.logger.Debug("Mount %s", glog.File(ofs.mergedDir))
	if !gutils.DirExists(ofs.mergedDir) {
		err := os.MkdirAll(ofs.mergedDir, 700)
		if err != nil {
			return fmt.Errorf("mkdir merged (%s): %w", ofs.mergedDir, err)
		}
		time.Sleep(DELAY_OVERLAYFS_MKDIR)
	}

	if !gutils.DirExists(ofs.workDir) {
		err := os.MkdirAll(ofs.workDir, 700)
		if err != nil {
			return fmt.Errorf("mkdir workdir (%s): %w", ofs.workDir, err)
		}
		time.Sleep(DELAY_OVERLAYFS_MKDIR)
	}

	if !gutils.DirExists(ofs.upperDir) {
		err := os.MkdirAll(ofs.upperDir, 700)
		if err != nil {
			return fmt.Errorf("mkdir upper (%s): %w", ofs.upperDir, err)
		}
		time.Sleep(DELAY_OVERLAYFS_MKDIR)
	}

	lowerdirs := strings.Join(ofs.lowerDirs, ":")
	data := fmt.Sprintf("lowerdir=%s,upperdir=%s,workdir=%s", lowerdirs, ofs.upperDir, ofs.workDir)

	if gutils.DirExists(ofs.mergedDir) {
		err := unix.Mount("overlay", ofs.mergedDir, "overlay", 0, data)
		if err != nil {
			return fmt.Errorf("mount (%s): %w", ofs.mergedDir, err)
		}
		return nil
	}

	return fmt.Errorf("mount (%s): %s", ofs.mergedDir, "the directory does not exist")
}

func (ofs *FakeFS) Close() {
	ofs.manager.DeactivateOverlay(ofs)
}

func (ofs *FakeFS) Unmount() error {
	ofs.logger.Debug("Unmount %s", glog.File(ofs.mergedDir))
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

func (ofs *FakeFS) insideMerged(path string) bool {
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

func (ofs *FakeFS) RemoveFile(path string, recursive bool) error {
	ofs.logger.Info("Remove %s%s", glog.File(ofs.mergedDir), glog.Reason(path))

	if !ofs.insideMerged(path) {
		return errors.New("path outside root")
	}
	if recursive {
		return os.RemoveAll(filepath.Join(ofs.mergedDir, path))
	}
	return os.Remove(filepath.Join(ofs.mergedDir, path))
}

func (ofs *FakeFS) OpenFile(path string, flag int, perm fs.FileMode) (*os.File, error) {
	ofs.logger.Info("Open %s%s", glog.File(ofs.mergedDir), glog.Reason(path))

	if !ofs.insideMerged(path) {
		return nil, errors.New("path outside root")
	}

	// create the directory structure, so we don't get not found errors
	// because our fake file system is incomplete
	_ = ofs.MkdirAll(filepath.Dir(path), perm)

	return os.OpenFile(filepath.Join(ofs.mergedDir, path), flag, perm)
}

func (ofs *FakeFS) DirExists(path string) bool {
	if !ofs.insideMerged(path) {
		return false
	}

	return gutils.DirExists(filepath.Join(ofs.mergedDir, path))
}

func (ofs *FakeFS) FileExists(path string) bool {
	if !ofs.insideMerged(path) {
		return false
	}

	return gutils.FileExists(filepath.Join(ofs.mergedDir, path))
}

func (ofs *FakeFS) Mkdir(path string, mode fs.FileMode) error {
	ofs.logger.Debug("Mkdir %s", glog.File(path))
	if !ofs.insideMerged(path) {
		return errors.New("path outside root")
	}

	return os.Mkdir(filepath.Join(ofs.mergedDir, path), mode)
}

func (ofs *FakeFS) MkdirAll(path string, mode fs.FileMode) error {
	ofs.logger.Debug("MkdirAll %s", glog.File(path))
	if !ofs.insideMerged(path) {
		return errors.New("path outside root")
	}

	return os.MkdirAll(filepath.Join(ofs.mergedDir, path), mode)
}

func (ofs *FakeFS) ReadDir(path string) ([]os.DirEntry, error) {
	ofs.logger.Debug("ReadDir %s", glog.File(path))
	if !ofs.insideMerged(path) {
		return nil, errors.New("path outside root")
	}

	return os.ReadDir(filepath.Join(ofs.mergedDir, path))
}
