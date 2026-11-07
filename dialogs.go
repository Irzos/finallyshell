package main

import (
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

const (
	authLabelPassword = "密码"
	authLabelKey      = "私钥"
)

// hostForm edits a host in the connection manager.
type hostForm struct {
	editing    *Host // nil for a new host
	name       string
	addr       string
	port       string
	user       string
	auth       string
	password   string
	keyPath    string
	passphrase string
	err        string
}

func (a *App) openManager() {
	a.managerOpen = true
	a.formOpen = len(a.hosts) == 0
	if a.formOpen {
		a.startForm(nil)
	}
}

func (a *App) startForm(h *Host) {
	a.form = hostForm{editing: h, port: "22", user: "root", auth: authLabelPassword}
	if h != nil {
		a.form.name, a.form.addr, a.form.user, a.form.keyPath = h.Name, h.Addr, h.User, h.KeyPath
		a.form.port = strconv.Itoa(h.port())
		if h.Auth == authKey {
			a.form.auth = authLabelKey
		}
		a.form.password = a.passwords[h.ID]
	}
	a.formOpen = true
}

func (a *App) dialogs(c *ui.Context) {
	ui.Modal(c, &a.managerOpen, func() {
		ui.Column(c).Width(520).Gap(12).Children(func() {
			if a.formOpen {
				a.hostFormView(c)
			} else {
				a.hostListView(c)
			}
		})
	})
	ui.Modal(c, &a.pwOpen, func() { a.passwordView(c) })
	if s := a.pendingHostKey(); s != nil {
		open := true
		ui.Modal(c, &open, func() { a.hostKeyView(c, s) })
		if !open && s.hostKeyQ != nil { // Escape or a click outside
			s.hostKeyQ.answer <- false
			s.hostKeyQ = nil
		}
	}
}

func (a *App) hostListView(c *ui.Context) {
	ui.Row(c).Children(func() {
		ui.Text(c, "连接管理器").FontSize(16).Bold().Grow(1)
		if ui.Button(c, "新建").Clicked() {
			a.startForm(nil)
		}
	})
	a.hostList.Selected = &a.hostRow
	a.hostList.Key = func(i int) any { return a.hosts[i].ID }
	a.hostList.Label = func(i int) string { return a.hosts[i].title() }
	list := ui.List(c, &a.hostList, len(a.hosts), func(i int) {
		h := a.hosts[i]
		sub := theme.TextMuted
		if i == a.hostRow {
			sub = theme.AccentText.Alpha(0.8)
		}
		ui.Row(c).Gap(10).Padding(6, 8).Children(func() {
			ui.Icon(c, icon("server")).FontSize(16).TextColor(sub)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, h.title()).SingleLine()
				ui.Text(c, h.target()).FontSize(11.5).TextColor(sub).SingleLine()
			})
		})
	}).Height(300).Border(1, theme.Border).Radius(6)
	if list.Submitted() && a.hostRow >= 0 && a.hostRow < len(a.hosts) {
		a.managerOpen = false
		a.openSession(a.hosts[a.hostRow])
	}
	chosen := a.hostRow >= 0 && a.hostRow < len(a.hosts)
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "编辑").Disabled(!chosen).Clicked() {
			a.startForm(a.hosts[a.hostRow])
		}
		if ui.Button(c, "删除").Disabled(!chosen).Clicked() {
			a.hosts = slices.Delete(a.hosts, a.hostRow, a.hostRow+1)
			a.hostRow = -1
			a.saveHostsFunc(a.hosts)
		}
		ui.Spacer(c)
		if ui.Button(c, "取消").Clicked() {
			a.managerOpen = false
		}
		if ui.PrimaryButton(c, "连接").Disabled(!chosen).Clicked() {
			a.managerOpen = false
			a.openSession(a.hosts[a.hostRow])
		}
	})
}

func field(c *ui.Context, label string, fn func()) {
	ui.Row(c).Gap(10).Children(func() {
		ui.Text(c, label).Width(72).Shrink(0)
		ui.Row(c).Grow(1).Gap(8).Children(fn)
	})
}

func (a *App) hostFormView(c *ui.Context) {
	f := &a.form
	title := "新建连接"
	if f.editing != nil {
		title = "编辑连接"
	}
	ui.Text(c, title).FontSize(16).Bold()
	field(c, "名称", func() { ui.TextInput(c, &f.name).Placeholder("可选，例如 生产服务器").Grow(1) })
	field(c, "主机", func() {
		ui.TextInput(c, &f.addr).Placeholder("IP 或域名").Grow(1).AutoFocus()
		ui.Text(c, "端口")
		ui.TextInput(c, &f.port).Width(64)
	})
	field(c, "用户名", func() { ui.TextInput(c, &f.user).Grow(1) })
	field(c, "认证方式", func() { ui.Select(c, &f.auth, []string{authLabelPassword, authLabelKey}).Width(120) })
	if f.auth == authLabelKey {
		field(c, "私钥文件", func() { ui.TextInput(c, &f.keyPath).Placeholder("~/.ssh/id_ed25519").Grow(1) })
		field(c, "私钥密码", func() { ui.TextInput(c, &f.passphrase).Password().Placeholder("没有可留空").Grow(1) })
	} else {
		field(c, "密码", func() { ui.TextInput(c, &f.password).Password().Grow(1) })
	}
	muted(c, "密码只保存在内存中，关闭软件后需要重新输入。").FontSize(12)
	if f.err != "" {
		ui.Text(c, f.err).TextColor(theme.Danger)
	}
	ui.Row(c).Gap(8).Children(func() {
		if len(a.hosts) > 0 && ui.Button(c, "返回").Clicked() {
			a.formOpen = false
		}
		ui.Spacer(c)
		if ui.Button(c, "保存").Clicked() {
			a.saveForm()
		}
		if ui.PrimaryButton(c, "保存并连接").Clicked() {
			if h := a.saveForm(); h != nil {
				a.managerOpen = false
				a.openSession(h)
			}
		}
	})
}

// saveForm checks the form and saves its host, returning it, or nil with
// the form's error set.
func (a *App) saveForm() *Host {
	f := &a.form
	f.addr, f.user = strings.TrimSpace(f.addr), strings.TrimSpace(f.user)
	port, err := strconv.Atoi(strings.TrimSpace(f.port))
	switch {
	case f.addr == "":
		f.err = "请填写主机"
		return nil
	case f.user == "":
		f.err = "请填写用户名"
		return nil
	case err != nil || port <= 0 || port > 65535:
		f.err = "端口不正确"
		return nil
	case f.auth == authLabelKey && strings.TrimSpace(f.keyPath) == "":
		f.err = "请填写私钥文件"
		return nil
	}
	h := f.editing
	if h == nil {
		h = &Host{ID: newHostID()}
		a.hosts = append(a.hosts, h)
	}
	h.Name, h.Addr, h.Port, h.User = strings.TrimSpace(f.name), f.addr, port, f.user
	if f.auth == authLabelKey {
		h.Auth, h.KeyPath = authKey, strings.TrimSpace(f.keyPath)
		a.passwords[h.ID] = f.passphrase
	} else {
		h.Auth, h.KeyPath = authPassword, ""
		a.passwords[h.ID] = f.password
	}
	if err := a.saveHostsFunc(a.hosts); err != nil {
		f.err = "保存失败：" + err.Error()
	} else {
		f.err = ""
	}
	a.formOpen = false
	a.hostRow = slices.Index(a.hosts, h)
	return h
}

func (a *App) passwordView(c *ui.Context) {
	h := a.pwHost
	if h == nil {
		return
	}
	ui.Column(c).Width(380).Gap(12).Children(func() {
		ui.Text(c, "输入密码").FontSize(16).Bold()
		muted(c, h.target())
		submit := ui.TextInput(c, &a.pwText).Password().AutoFocus().Submitted()
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "取消").Clicked() {
				a.pwOpen = false
			}
			if ui.PrimaryButton(c, "连接").Clicked() {
				submit = true
			}
		})
		if submit {
			a.passwords[h.ID] = a.pwText
			a.pwOpen, a.pwText = false, ""
			a.openSession(h)
		}
	})
}

func (a *App) pendingHostKey() *Session {
	for _, s := range a.sessions {
		if s.hostKeyQ != nil {
			return s
		}
	}
	return nil
}

func (a *App) hostKeyView(c *ui.Context, s *Session) {
	q := s.hostKeyQ
	ui.Column(c).Width(460).Gap(10).Children(func() {
		ui.Text(c, "首次连接到这台主机").FontSize(16).Bold()
		ui.Text(c, "无法确认 "+q.host+" 的身份。请核对下面的指纹与服务器上的一致，再选择信任。")
		ui.Column(c).Gap(4).Padding(8).Radius(6).Background(ui.Hex("#f3f4f6")).Children(func() {
			muted(c, q.keyType)
			ui.Text(c, q.fingerprint).Font("monospace").Selectable()
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "拒绝").Clicked() {
				q.answer <- false
				s.hostKeyQ = nil
			}
			if ui.PrimaryButton(c, "信任并连接").Clicked() {
				q.answer <- true
				s.hostKeyQ = nil
			}
		})
	})
}
