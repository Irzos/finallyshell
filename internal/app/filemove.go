package app

import (
	"path"
	"slices"
	"strings"

	"finallyshell/internal/remotefs"
	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
	"github.com/pkg/sftp"
)

// fileDrag carries names from one remote directory on one connection.
type fileDrag struct {
	client *sftp.Client
	dir    string
	names  []string
}

func (d fileDrag) canMoveTo(s *Session, dir string) bool {
	if s.sftp == nil || d.client != s.sftp || s.files.moving || dir == "" || dir == d.dir {
		return false
	}
	for _, name := range d.names {
		if within(dir, path.Join(d.dir, name)) {
			return false
		}
	}
	return true
}

func (a *App) fileMoveDropZone(e ui.Element, s *Session, dir string) {
	if d, ok := ui.DragOver[fileDrag](e); ok && d.canMoveTo(s, dir) {
		e.DrawOver(func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, theme.Accent.Alpha(0.18), 0)
			p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - 2, W: r.W, H: 2}, theme.Accent, 0)
		})
	}
	if d, ok := ui.Drop[fileDrag](e); ok && d.canMoveTo(s, dir) {
		a.moveRemoteFiles(s, d, dir)
	}
}

// moveRemoteFiles stops at the first failure and refreshes any items
// already moved.
func (a *App) moveRemoteFiles(s *Session, d fileDrag, dir string) {
	f := &s.files
	f.moving = true
	go func() {
		moved := 0
		var err error
		for _, name := range d.names {
			if err = remotefs.Move(d.client, path.Join(d.dir, name), dir); err != nil {
				break
			}
			moved++
		}
		a.post(func() {
			f.moving = false
			if s.sftp != d.client {
				return
			}
			if moved > 0 {
				a.remoteFilesMoved(s, d.dir, dir, d.names[:moved])
			}
			if err != nil {
				a.notify(locales.CannotMoveFile(d.names[moved], dir, fileOpErrorText(err)))
			}
		})
	}()
}

func (a *App) remoteFilesMoved(s *Session, from, to string, names []string) {
	f := &s.files
	cwd := f.cwd
	if f.loading {
		cwd = f.listingDir
	}
	next := cwd
	for _, name := range names {
		old, moved := path.Join(from, name), path.Join(to, name)
		f.moveTree(old, moved)
		if within(cwd, old) {
			next = moved + strings.TrimPrefix(cwd, old)
		}
		if f.cwd == from {
			f.chosen.Remove(name)
		}
	}
	if f.cwd == from && f.row >= 0 && f.row < len(f.entries) && slices.Contains(names, f.entries[f.row].Name) {
		f.row = -1
	}
	f.forgetSubdirs(from)
	f.forgetSubdirs(to)
	if next != cwd || cwd == from || cwd == to {
		a.openDir(s, next)
	}
}
