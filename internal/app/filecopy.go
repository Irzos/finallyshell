package app

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"finallyshell/internal/remotefs"
	"finallyshell/locales"

	"github.com/egoist/mygo/transfer"
)

const pendingFileCopy transfer.Format = "application/vnd.finallyshell.file-copy"
const gnomeCopiedFiles transfer.Format = "x-special/gnome-copied-files"

// Reserve the clipboard at the moment the user copies. Publishing later
// requires this token so a newer copy, including one in another app, wins.
func (a *App) reserveFileClipboard() (string, error) {
	token := rand.Text()
	err := a.plat.WriteClipboardData(transfer.New(transfer.NewItem(
		transfer.Bytes(pendingFileCopy, []byte(token)),
	)))
	return token, err
}

func (a *App) copyRemoteFiles(s *Session, names []string) {
	if s.sftp == nil || s.files.cwd == "" || len(names) == 0 {
		return
	}
	srcs := make([]string, len(names))
	for i, name := range names {
		if path.Base(name) != name || filepath.Base(name) != name || !filepath.IsLocal(name) || name == "." {
			a.notify(locales.FileCopyFailed(locales.BadFileName()))
			return
		}
		srcs[i] = path.Join(s.files.cwd, name)
	}
	token, err := a.reserveFileClipboard()
	if err != nil {
		a.notify(locales.FileCopyFailed(err.Error()))
		return
	}
	a.enqueue(&transferJob{
		sess: s, sftp: s.sftp, dir: download, src: srcs[0], srcs: srcs,
		dstDir: filepath.Join(a.plat.TempDir(), "clipboard"), clipboard: token,
	})
	a.notify(locales.FileCopyPreparing())
}

// stageFileCopy downloads the whole selection before exposing any of it
// to a file manager. A fresh directory isolates each attempt and retry.
func (a *App) stageFileCopy(ctx context.Context, t *transferJob, report remotefs.ArchiveProgress) (string, error) {
	root := t.dstDir
	a.transfers.pruneClipboard.Do(func() { removeOld(root, 24*time.Hour) })
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, "")
	if err != nil {
		return "", err
	}
	keep := false
	defer func() {
		if !keep {
			os.RemoveAll(dir)
		}
	}()
	var total int64
	for _, src := range t.srcs {
		size, err := remotefs.RemoteSizeContext(ctx, t.sftp, src)
		if err != nil {
			return "", err
		}
		total += size
	}
	a.post(func() {
		t.total = total
		a.showTransfers()
	})
	var done int64
	for _, src := range t.srcs {
		var copied int64
		_, err := remotefs.DownloadNew(ctx, t.sftp, src, dir, func(n int64) {
			copied = n
			report(done+n, "")
		})
		if err != nil {
			return "", err
		}
		done += copied
	}
	keep = true
	return dir, nil
}

func (a *App) finishFileCopy(ctx context.Context, t *transferJob, dir string, err error) error {
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = a.publishFileCopy(t, dir)
	}
	switch {
	case err == nil:
		a.notify(locales.FileCopyReady())
	case !remotefs.IsCanceled(err):
		a.notify(locales.FileCopyFailed(transferErrorText(err)))
	}
	if err != nil && dir != "" {
		go os.RemoveAll(dir)
	}
	return err
}

func (a *App) publishFileCopy(t *transferJob, dir string) error {
	paths := make([]string, len(t.srcs))
	for i, src := range t.srcs {
		paths[i] = filepath.Join(dir, path.Base(src))
	}
	data, err := fileClipboardData(paths, runtime.GOOS)
	if err != nil {
		return err
	}
	token, err := a.plat.ReadClipboardFormat(pendingFileCopy)
	if err != nil || string(token) != t.clipboard {
		return errors.New(locales.FileCopyClipboardChanged())
	}
	return a.plat.WriteClipboardData(data)
}

func fileClipboardData(paths []string, goos string) (transfer.Data, error) {
	data, err := transfer.FileData(paths...)
	if err != nil || goos != "linux" || len(paths) == 0 {
		return data, err
	}
	// GNOME Files also looks for a copy/cut action followed by the URIs.
	// Other Linux file managers can use the standard MIME file list.
	uris, err := data.URLs()
	if err != nil {
		return transfer.Data{}, err
	}
	items := data.Items()
	items[0] = transfer.NewItem(
		transfer.Bytes(transfer.FileList, []byte(uris[0])),
		transfer.Bytes(transfer.URIList, []byte(uris[0])),
		transfer.Bytes(gnomeCopiedFiles, []byte("copy\n"+strings.Join(uris, "\n"))),
	)
	return transfer.New(items...), nil
}
