package main

import (
	"fmt"
	"path"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

var termBG = ui.Hex("#1d1f21") // Tomorrow Night's background

// terminalArea shows the active session's terminal, or what keeps it
// from showing.
func (a *App) terminalArea(c *ui.Context) {
	s := a.current()
	if s == nil {
		a.welcome(c)
		return
	}
	ui.Column(c.Key(s.ID)).Fill().Background(termBG).Children(func() {
		if s.status != statusConnected {
			a.statusBanner(c, s)
		}
		if s.term != nil {
			ui.Box(c).Grow(1).Padding(4, 0, 0, 6).Children(func() {
				terminal.View(c, s.term).Fill().AutoFocus()
			})
			return
		}
		ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
			switch s.status {
			case statusConnecting:
				ui.Spinner(c).TextColor(ui.Hex("#c5c8c6")).FontSize(22)
				ui.Text(c, "正在连接 "+s.Host.target()).TextColor(ui.Hex("#c5c8c6"))
			default:
				ui.Text(c, statusText(s)).TextColor(ui.Hex("#c5c8c6")).FontSize(15)
			}
		})
	})
}

// statusBanner is the bar over the terminal of a session that is not
// connected, with what went wrong and a button to connect again.
func (a *App) statusBanner(c *ui.Context, s *Session) {
	if s.status == statusConnecting && s.term == nil {
		return
	}
	bg := ui.Hex("#7f1d1d")
	if s.status == statusConnecting {
		bg = ui.Hex("#713f12")
	}
	ui.Row(c).Background(bg).Padding(6, 10).Gap(10).Children(func() {
		msg := statusText(s)
		if s.err != "" {
			msg = s.err
		}
		ui.Text(c, msg).TextColor(ui.Hex("#fef2f2")).Grow(1).MinWidth(0)
		if s.status != statusConnecting && ui.Button(c, "重新连接").Clicked() {
			a.reconnect(s)
		}
	})
}

func (a *App) welcome(c *ui.Context) {
	ui.Column(c).Fill().Background(termBG).Center().Gap(14).Children(func() {
		ui.Icon(c, icon("terminal")).FontSize(40).TextColor(ui.Hex("#6b7280"))
		ui.Text(c, "没有打开的会话").TextColor(ui.Hex("#c5c8c6")).FontSize(16)
		if ui.PrimaryButton(c, "新建连接").Clicked() {
			a.openManager()
		}
		muted(c, "Ctrl+Shift+T 打开连接管理器")
	})
}

var fileColumns = []ui.TableColumn{
	{Title: "文件名", MinWidth: 160},
	{Title: "大小", Width: 90, Align: ui.End},
	{Title: "类型", Width: 90},
	{Title: "修改时间", Width: 140},
	{Title: "权限", Width: 100},
	{Title: "用户/用户组", Width: 90},
}

// filePanel browses the active session's server over SFTP.
func (a *App) filePanel(c *ui.Context) {
	s := a.current()
	ui.Column(c).Fill().Background(theme.Background).Children(func() {
		if s == nil {
			return
		}
		f := &s.files
		ui.Column(c.Key(s.ID)).Fill().Children(func() {
			ui.Row(c).Padding(4, 8).Gap(4).BorderWidth(0, 0, 1, 0).BorderColor(ui.Hex("#e5e7eb")).Children(func() {
				if ui.TextInput(c, &f.pathEdit).Grow(1).Submitted() {
					a.openDir(s, f.pathEdit)
				}
				if iconButton(c, "arrow-up", "上级目录").Clicked() && f.cwd != "" {
					a.openDir(s, path.Dir(f.cwd))
				}
				if iconButton(c, "refresh-cw", "刷新").Clicked() && f.cwd != "" {
					a.openDir(s, f.cwd)
				}
				if iconButton(c, "house", "主目录").Clicked() && s.sftp != nil {
					if home, err := s.sftp.Getwd(); err == nil {
						a.openDir(s, home)
					}
				}
				if f.loading {
					ui.Spinner(c)
				}
			})
			if f.err != "" {
				ui.Text(c, f.err).TextColor(theme.Danger).Padding(4, 10)
			}
			ui.Split(c, &a.treeWidth, func() { a.dirTree(c, s) }, func() { a.fileTable(c, s) }).Grow(1)
		})
	})
}

func (a *App) dirTree(c *ui.Context, s *Session) {
	f := &s.files
	f.tree.List.Selected = &f.treeRow
	tree := ui.Outline(c, &f.tree, []string{"/"}, func(dir string) []string {
		subs, loaded := f.subdirs[dir]
		if !loaded {
			if f.tree.Open.Has(dir) {
				a.loadSubdirs(s, dir)
			}
			return []string{}
		}
		if len(subs) == 0 {
			return nil
		}
		return subs
	}, func(dir string) {
		name := path.Base(dir)
		ui.Row(c).Gap(5).Children(func() {
			ui.Icon(c, icon("folder")).FontSize(14).TextColor(ui.Hex("#eab308"))
			t := ui.Text(c, name).SingleLine()
			if dir == f.cwd {
				t.Bold()
			}
		})
	}).Fill()
	if tree.Changed() {
		if dir := f.tree.Item(f.treeRow); dir != "" && dir != f.cwd {
			a.openDir(s, dir)
		}
	}
}

func (a *App) fileTable(c *ui.Context, s *Session) {
	f := &s.files
	entries := f.entries
	f.table.Selected = &f.row
	f.table.Key = func(i int) any { return entries[i].Name }
	f.table.Label = func(i int) string { return entries[i].Name }
	t := ui.Table(c, &f.table, fileColumns, len(entries), func(row, col int) {
		e := entries[row]
		switch col {
		case 0:
			ui.Row(c).Gap(6).Children(func() {
				ic, color := icon("file"), theme.TextMuted
				if e.dir() {
					ic, color = icon("folder"), ui.Hex("#eab308")
				}
				ui.Icon(c, ic).FontSize(14).TextColor(color)
				ui.Text(c, e.Name).SingleLine()
			})
		case 1:
			if !e.dir() {
				ui.Text(c, formatSize(e.Size))
			}
		case 2:
			ui.Text(c, e.kind()).SingleLine()
		case 3:
			ui.Text(c, e.ModTime.Format("2006/01/02 15:04")).SingleLine()
		case 4:
			ui.Text(c, e.Mode.Perm().String()).Font("monospace")
		case 5:
			ui.Text(c, fmt.Sprintf("%d/%d", e.UID, e.GID))
		}
	}).Fill()
	if t.Submitted() && f.row >= 0 && f.row < len(entries) && entries[f.row].dir() {
		a.openDir(s, path.Join(f.cwd, entries[f.row].Name))
	}
	t.ContextMenu(func(m *ui.Menu) {
		if m.Item("刷新").Chosen() {
			a.openDir(s, f.cwd)
		}
		if f.row >= 0 && f.row < len(entries) && m.Item("复制路径").Chosen() {
			c.WriteClipboard(path.Join(f.cwd, entries[f.row].Name))
		}
	})
}
