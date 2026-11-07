package main

import (
	"embed"
	"slices"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// App is the state of the window.
type App struct {
	// post runs fn on the UI thread and redraws: Window.Update in the app,
	// a direct call in tests.
	post func(fn func())

	hosts     []*Host
	passwords map[string]string // by host ID, for this run only
	sessions  []*Session
	active    int
	sessList  ui.ListState

	sideWidth     float32
	monitorPct    float32
	terminalPct   float32
	treeWidth     float32
	termFont      terminal.Font
	termTheme     *terminal.Theme
	saveHostsFunc func([]*Host) error

	// The connection manager.
	managerOpen bool
	hostRow     int
	hostList    ui.ListState
	form        hostForm
	formOpen    bool

	// The password prompt for a saved host.
	pwOpen bool
	pwHost *Host
	pwText string
	pwErr  string
}

func newApp(post func(func())) *App {
	a := &App{
		post:          post,
		passwords:     map[string]string{},
		active:        -1,
		hostRow:       -1,
		sideWidth:     236,
		monitorPct:    62,
		terminalPct:   66,
		treeWidth:     200,
		termFont:      terminal.Font{Family: "JetBrains Mono, Cascadia Mono, Consolas, Menlo, DejaVu Sans Mono, monospace", Size: 13.5},
		saveHostsFunc: saveHosts,
	}
	if t, err := terminal.GhosttyTheme("Tomorrow Night"); err == nil {
		a.termTheme = t
	}
	a.hosts, _ = loadHosts()
	return a
}

func (a *App) current() *Session {
	if a.active >= 0 && a.active < len(a.sessions) {
		return a.sessions[a.active]
	}
	return nil
}

func appTheme() *ui.Theme {
	t := ui.LightTheme()
	t.Spacing = 3
	t.FontSize = 13
	t.Font = "Microsoft YaHei UI, PingFang SC, Noto Sans CJK SC, WenQuanYi Zen Hei, system-ui"
	return t
}

var theme = appTheme()

// sidebarBG is the gray of the side panel, as FinalShell's.
var sidebarBG = ui.Hex("#f3f4f6")

func (a *App) view(c *ui.Context) {
	c.SetTheme(theme)
	a.shortcuts(c)
	ui.Split(c, &a.sideWidth, func() { a.sidebar(c) }, func() { a.mainArea(c) }).SplitReset(236).Fill()
	a.dialogs(c)
}

func (a *App) shortcuts(c *ui.Context) {
	switch {
	case c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyT), c.Shortcut(ui.Cmd, ui.KeyT):
		a.openManager()
	case c.Shortcut(ui.Ctrl, ui.KeyTab):
		a.cycle(1)
	case c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyTab):
		a.cycle(-1)
	case c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyW):
		if s := a.current(); s != nil {
			a.closeSession(s)
		}
	}
}

func (a *App) cycle(d int) {
	if n := len(a.sessions); n > 0 {
		a.active = ((a.active+d)%n + n) % n
	}
}

func (a *App) sidebar(c *ui.Context) {
	ui.Column(c).Fill().Background(sidebarBG).Children(func() {
		ui.SplitVerticalPercent(c, &a.monitorPct, func() { a.monitorPanel(c) }, func() { a.sessionPanel(c) }).Fill()
	})
}

func (a *App) mainArea(c *ui.Context) {
	ui.SplitVerticalPercent(c, &a.terminalPct, func() { a.terminalArea(c) }, func() { a.filePanel(c) }).Fill()
}

// openSession opens a tab for h, asking for its password first when the
// app does not know it.
func (a *App) openSession(h *Host) {
	pw, known := a.passwords[h.ID]
	if h.Auth == authPassword && !known {
		a.pwOpen, a.pwHost, a.pwText, a.pwErr = true, h, "", ""
		return
	}
	s := newSession(h)
	a.sessions = append(a.sessions, s)
	a.active = len(a.sessions) - 1
	opts := dialOptions{password: pw}
	if h.Auth == authKey {
		opts.passphrase = pw
	}
	a.connect(s, opts)
}

func (a *App) reconnect(s *Session) {
	s.disconnect()
	h := s.Host
	pw := a.passwords[h.ID]
	opts := dialOptions{password: pw}
	if h.Auth == authKey {
		opts.passphrase = pw
	}
	a.connect(s, opts)
}

func (a *App) closeSession(s *Session) {
	i := slices.Index(a.sessions, s)
	if i < 0 {
		return
	}
	s.disconnect()
	a.sessions = slices.Delete(a.sessions, i, i+1)
	if a.active >= len(a.sessions) || a.active > i {
		a.active--
	}
	if a.active < 0 && len(a.sessions) > 0 {
		a.active = 0
	}
}

func (a *App) closeAll() {
	for _, s := range a.sessions {
		s.disconnect()
	}
}

func (a *App) moveSessions(rows []int, to int) {
	cur := a.current()
	moved := make([]*Session, 0, len(rows))
	for _, r := range rows {
		moved = append(moved, a.sessions[r])
	}
	var rest []*Session
	insert := 0
	for i, s := range a.sessions {
		if slices.Contains(moved, s) {
			continue
		}
		if i < to {
			insert++
		}
		rest = append(rest, s)
	}
	a.sessions = slices.Insert(rest, insert, moved...)
	a.active = slices.Index(a.sessions, cur)
}

//go:embed assets/icons/*.svg
var iconFS embed.FS

var icons = map[string]*ui.SVG{}

func icon(name string) *ui.SVG {
	if s, ok := icons[name]; ok {
		return s
	}
	data, err := iconFS.ReadFile("assets/icons/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	s := ui.MustParseSVG(data)
	icons[name] = s
	return s
}

// iconButton is a small square button showing an icon, named by label.
func iconButton(c *ui.Context, name, label string) ui.Element {
	b := ui.Box(c.Key(label)).Size(24, 24).Center().Radius(5).Role(ui.RoleButton).Label(label).Tooltip(label).Shrink(0)
	if b.Hovered() {
		b.Background(ui.RGBA(0, 0, 0, 0.07))
	}
	b.Children(func() {
		ui.Icon(c, icon(name)).FontSize(15).TextColor(theme.TextMuted)
	})
	return b
}

func muted(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).TextColor(theme.TextMuted)
}

func trimTo(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}
