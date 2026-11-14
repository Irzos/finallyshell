package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"finallyshell/locales"

	"github.com/egoist/mygo/ui"
)

func TestDragRemoteFiles(t *testing.T) {
	f := newEditorFixture(t)
	tt, files := f.tt, &f.s.files
	f.a.terminalPct = 45
	for _, dir := range []string{"incoming", "source/subfolder"} {
		if err := os.MkdirAll(filepath.Join(f.dir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range []string{"", "incoming"} {
		if err := os.WriteFile(filepath.Join(f.dir, dir, "z-conflict"), []byte(dir), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f.a.openDir(f.s, f.dir)
	f.settled(t)

	x, y := rowCenter(t, tt, "app.yaml")
	tx, ty := rowCenter(t, tt, "incoming")
	if os.Getenv("SHOT_DIR") != "" {
		for _, lang := range []string{"en", "zh-Hans"} {
			f.p.SetLocale(lang)
			tt.Frame()
			tt.Press(x, y)
			tt.Move(tx+200, ty)
			tt.Frame()
			saveShot(t, tt, "remote-file-drag-"+lang+".png")
			tt.Key(0, ui.KeyEscape)
			tt.Release(tx+200, ty)
			tt.Frame()
		}
		f.p.SetLocale("en")
		tt.Frame()
	}

	// Dropping a selection on a folder row moves it until a name conflicts.
	tt.ClickAt(x, y)
	zx, zy := rowCenter(t, tt, "z-conflict")
	tt.ClickAtWith(ui.Cmd, zx, zy)
	if got := files.chosenNames(); !slices.Equal(got, []string{"app.yaml", "z-conflict"}) {
		t.Fatalf("selection before dragging = %q", got)
	}
	dragTo(tt, x, y, tx, ty)
	f.p.until(t, "the move and conflict", func() bool {
		return !files.moving && !files.loading && tt.HasText(locales.FileNameTaken("z-conflict"))
	})
	if f.exists("app.yaml") || f.read(t, "incoming/app.yaml") != appYAML ||
		f.read(t, "z-conflict") != "" || f.read(t, "incoming/z-conflict") != "incoming" {
		t.Fatal("the move or the conflict changed the wrong files")
	}
	if got := files.chosenNames(); !slices.Equal(got, []string{"z-conflict"}) {
		t.Errorf("selection after a partial move = %q", got)
	}

	// The tree accepts folder moves and displays the changed destination.
	incoming := filepath.Join(f.dir, "incoming")
	source := filepath.Join(f.dir, "source")
	f.a.openDir(f.s, incoming)
	f.settled(t)
	f.a.openDir(f.s, source)
	f.settled(t)
	files.tree.Open.Remove(source)
	// Keep the fixture's three folders visible regardless of how many
	// siblings the server's working directory has.
	files.subdirs["/"] = []string{f.dir}
	tt.Frame()
	files.tree.List.ScrollTo(0, ui.Start)
	f.showInTree(t, incoming)
	sx, sy := rowCenter(t, tt, "subfolder")
	tx, ty = rowCenter(t, tt, "incoming")
	if tx >= sx {
		t.Fatal("the drop target should be in the folder tree")
	}
	dragTo(tt, sx, sy, sx+80, sy)
	if files.moving || !f.exists("source/subfolder") {
		t.Fatal("a folder accepted a drop onto itself")
	}
	dragTo(tt, sx, sy, tx, ty)
	if !files.moving {
		t.Fatal("the directory tree did not accept the drop")
	}
	f.p.until(t, "the updated folder tree", func() bool {
		return !files.moving && !files.loading && slices.Contains(files.subdirs[incoming], filepath.Join(incoming, "subfolder"))
	})
	if !f.exists("incoming/subfolder") || f.exists("source/subfolder") || !f.exists("z-conflict") {
		t.Fatal("dragging the folder did not move just that folder")
	}
}
