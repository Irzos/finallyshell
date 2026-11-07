package main

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/pkg/sftp"
)

type fileEntry struct {
	Name    string
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
	UID     uint32
	GID     uint32
	Link    bool
}

func (e fileEntry) dir() bool { return e.Mode.IsDir() }

func (e fileEntry) kind() string {
	switch {
	case e.dir():
		return "文件夹"
	case e.Link:
		return "链接"
	}
	if ext := path.Ext(e.Name); ext != "" && len(ext) <= 8 && strings.Trim(ext[1:], "0123456789") != "" {
		return strings.ToUpper(ext[1:]) + " 文件"
	}
	return "文件"
}

// fileState is the file panel of a session.
type fileState struct {
	cwd      string
	pathEdit string
	entries  []fileEntry
	loading  bool
	err      string
	table    ui.ListState
	row      int

	tree     ui.OutlineState[string]
	treeRow  int
	subdirs  map[string][]string // the directories in each loaded directory
	treeBusy map[string]bool
}

func (f *fileState) init() {
	f.subdirs = map[string][]string{}
	f.treeBusy = map[string]bool{}
	f.row = -1
	f.treeRow = -1
	f.tree.Open.Add("/")
}

// openDir lists dir in the table and makes it the current directory.
func (a *App) openDir(s *Session, dir string) {
	if s.sftp == nil {
		return
	}
	dir = path.Clean(dir)
	f := &s.files
	f.loading, f.err = true, ""
	f.pathEdit = dir
	sc := s.sftp
	go func() {
		entries, err := readDir(sc, dir)
		a.post(func() {
			f.loading = false
			if err != nil {
				f.err = err.Error()
				f.pathEdit = f.cwd
				return
			}
			f.cwd, f.pathEdit, f.entries, f.row = dir, dir, entries, -1
			f.subdirs[dir] = dirNames(dir, entries)
			// Open the tree down to the directory.
			for p := dir; ; p = path.Dir(p) {
				f.tree.Open.Add(p)
				if p == "/" {
					break
				}
			}
			for p := path.Dir(dir); p != "/" && f.subdirs[p] == nil; p = path.Dir(p) {
				a.loadSubdirs(s, p)
			}
			if dir != "/" && f.subdirs["/"] == nil {
				a.loadSubdirs(s, "/")
			}
		})
	}()
}

// loadSubdirs reads the directories in dir, for the tree.
func (a *App) loadSubdirs(s *Session, dir string) {
	f := &s.files
	if s.sftp == nil || f.treeBusy[dir] {
		return
	}
	f.treeBusy[dir] = true
	sc := s.sftp
	go func() {
		entries, err := readDir(sc, dir)
		a.post(func() {
			delete(f.treeBusy, dir)
			if err != nil {
				f.subdirs[dir] = []string{}
				return
			}
			f.subdirs[dir] = dirNames(dir, entries)
		})
	}()
}

func readDir(sc *sftp.Client, dir string) ([]fileEntry, error) {
	infos, err := sc.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("无法打开 %s：%w", dir, err)
	}
	entries := make([]fileEntry, 0, len(infos))
	for _, fi := range infos {
		e := fileEntry{Name: fi.Name(), Size: fi.Size(), Mode: fi.Mode(), ModTime: fi.ModTime()}
		if st, ok := fi.Sys().(*sftp.FileStat); ok {
			e.UID, e.GID = st.UID, st.GID
		}
		if fi.Mode()&fs.ModeSymlink != 0 {
			e.Link = true
			// Show links to directories as directories, as FinalShell does.
			if target, err := sc.Stat(path.Join(dir, fi.Name())); err == nil && target.IsDir() {
				e.Mode = target.Mode()
			}
		}
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(x, y fileEntry) int {
		if x.dir() != y.dir() {
			if x.dir() {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(x.Name), strings.ToLower(y.Name))
	})
	return entries, nil
}

func dirNames(dir string, entries []fileEntry) []string {
	names := []string{}
	for _, e := range entries {
		if e.dir() {
			names = append(names, path.Join(dir, e.Name))
		}
	}
	return names
}

func formatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n)
	for _, u := range []string{"KB", "MB", "GB", "TB"} {
		f /= unit
		if f < unit {
			return fmt.Sprintf("%.1f %s", f, u)
		}
	}
	return fmt.Sprintf("%.1f PB", f/unit)
}

// formatBytes is a short size for the side panel: 2.7G, 113M, 65K.
func formatBytes(n float64) string {
	units := []string{"B", "K", "M", "G", "T"}
	i := 0
	for n >= 1024 && i < len(units)-1 {
		n /= 1024
		i++
	}
	if i == 0 || n >= 100 {
		return fmt.Sprintf("%.0f%s", n, units[i])
	}
	return fmt.Sprintf("%.1f%s", n, units[i])
}
