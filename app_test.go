package main

import (
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// idleConn is a connection that never sends anything, for a terminal the
// test writes to with Feed.
type idleConn struct{ done chan struct{} }

func (c idleConn) Read([]byte) (int, error)    { <-c.done; return 0, io.EOF }
func (c idleConn) Write(p []byte) (int, error) { return len(p), nil }
func (c idleConn) Close() error                { return nil }

func testApp(t *testing.T) *App {
	t.Setenv("GOSSH_DIR", t.TempDir())
	a := newApp(func(fn func()) { fn() })
	a.saveHostsFunc = func([]*Host) error { return nil }
	return a
}

// demoSession is a connected session with made-up stats and files.
func demoSession(t *testing.T, a *App, name, addr string) *Session {
	h := &Host{ID: newHostID(), Name: name, Addr: addr, User: "root", Auth: authPassword}
	a.hosts = append(a.hosts, h)
	s := newSession(h)
	s.status, s.ip = statusConnected, addr
	hist := func(base float64) []float64 {
		var h []float64
		for i := range historyLen {
			h = append(h, base*(1+0.6*float64((i*7)%11)/10))
		}
		return h
	}
	s.stats = Stats{
		OK: true, Uptime: 410 * 24 * time.Hour, Load: [3]float64{0.28, 0.31, 0.27}, CPU: 7,
		MemUsed: 2_900_000_000, MemTotal: 4_080_000_000, SwapUsed: 34_000_000, SwapTotal: 2_360_000_000,
		Procs: []Proc{{1_930_000_000, 3.3, "java"}, {118_000_000, 1.7, "mysqld"}, {467_000_000, 0.3, "syncthing"}, {1_300_000, 0.3, "top"}},
		Iface: "eth0", RxRate: 126_000, TxRate: 66_000, RxHist: hist(90_000), TxHist: hist(40_000),
		Latency: 254 * time.Millisecond, LatHist: hist(250),
		Disks: []Disk{{"/", 4_700_000_000, 25_000_000_000}, {"/dev/shm", 2_000_000_000, 2_000_000_000}},
	}
	f := &s.files
	f.cwd, f.pathEdit = "/lib64", "/lib64"
	mod := time.Date(2018, 6, 19, 21, 27, 0, 0, time.Local)
	f.entries = []fileEntry{
		{Name: "security", Mode: fs.ModeDir | 0o755, ModTime: mod},
		{Name: "ld-2.12.so", Size: 159_334, Mode: 0o755, ModTime: mod},
		{Name: "ld-linux-x86-64.so.2", Size: 159_334, Mode: 0o777, ModTime: mod, Link: true},
		{Name: "libacl.so.1", Size: 31_280, Mode: 0o755, ModTime: mod},
		{Name: "libaio.so.1.0.1", Size: 3_944, Mode: 0o755, ModTime: mod},
		{Name: "libanl.so.1", Size: 19_816, Mode: 0o755, ModTime: mod},
	}
	f.subdirs["/"] = []string{"/boot", "/etc", "/home", "/lib", "/lib64", "/root", "/usr"}
	f.subdirs["/lib64"] = []string{"/lib64/security"}
	f.tree.Open.Add("/lib64")
	a.sessions = append(a.sessions, s)
	return s
}

func saveShot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("SHOT_DIR")
	if dir == "" {
		return
	}
	out, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err := png.Encode(out, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestLayout(t *testing.T) {
	a := testApp(t)
	s := demoSession(t, a, "生产服务器", "45.56.78.90")
	demoSession(t, a, "测试机", "10.0.0.12").status = statusConnecting
	failed := demoSession(t, a, "备份服务器", "10.0.0.30")
	failed.status, failed.err = statusFailed, "dial tcp 10.0.0.30:22: i/o timeout"
	a.active = 0

	done := make(chan struct{})
	defer close(done)
	term, err := terminal.New(terminal.Options{Conn: idleConn{done}, Font: a.termFont, Theme: a.termTheme, DarkTheme: a.termTheme})
	if err != nil {
		t.Logf("no terminal (%v): the screenshot shows the connecting page", err)
	} else {
		defer term.Close()
		s.term = term
		term.Feed([]byte("Last login: Fri Nov  6 21:03:11 2026 from 1.2.3.4\r\n" +
			"[root@li900-223 ~]# ls -l /usr/bin | tail -4\r\n" +
			"-rwxr-xr-x  1 root root   1731 3月  22 2017 \x1b[1;32mzless\x1b[0m\r\n" +
			"-rwxr-xr-x  1 root root   2605 3月  22 2017 \x1b[1;32mzmore\x1b[0m\r\n" +
			"-rwxr-xr-x  1 root root   5246 3月  22 2017 \x1b[1;32mznew\x1b[0m\r\n" +
			"lrwxrwxrwx  1 root root      6 3月   9 2014 \x1b[1;36mzsoelim\x1b[0m -> \x1b[1;32msoelim\x1b[0m\r\n" +
			"[root@li900-223 ~]# "))
	}

	tt := ui.NewTester(a.view, 1280, 820)
	tt.Frame()
	time.Sleep(200 * time.Millisecond)
	tt.Frame()
	for _, want := range []string{"会话 (3)", "生产服务器", "测试机", "备份服务器", "IP 45.56.78.90", "运行 410 天", "java", "libacl.so.1", "文件名"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q; texts %q", want, tt.Texts())
		}
	}
	saveShot(t, tt, "layout.png")

	// The failed session shows its error and a button to reconnect.
	a.active = 2
	tt.Frame()
	if !tt.HasText("重新连接") {
		t.Errorf("no reconnect button; texts %q", tt.Texts())
	}
	saveShot(t, tt, "failed.png")
}

func TestConnectionManager(t *testing.T) {
	a := testApp(t)
	tt := ui.NewTester(a.view, 1280, 820)
	if err := tt.Click("新建连接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.managerOpen || !a.formOpen {
		t.Fatalf("manager %v form %v", a.managerOpen, a.formOpen)
	}
	a.form.addr = "example.com"
	if err := tt.Click("保存"); err != nil {
		t.Fatal(err)
	}
	if len(a.hosts) != 1 || a.hosts[0].Addr != "example.com" || a.hosts[0].port() != 22 {
		t.Fatalf("hosts %+v", a.hosts)
	}
	tt.Frame()
	if !tt.HasText("连接管理器") || !tt.HasText("root@example.com") {
		t.Errorf("list not shown; texts %q", tt.Texts())
	}
	saveShot(t, tt, "manager.png")
}

func TestMoveSessions(t *testing.T) {
	a := testApp(t)
	x := demoSession(t, a, "x", "1")
	y := demoSession(t, a, "y", "2")
	z := demoSession(t, a, "z", "3")
	a.active = 0
	a.moveSessions([]int{0}, 3)
	if a.sessions[0] != y || a.sessions[1] != z || a.sessions[2] != x || a.current() != x {
		t.Fatalf("order %v %v %v, active %d", a.sessions[0].Host.Name, a.sessions[1].Host.Name, a.sessions[2].Host.Name, a.active)
	}
}

func TestParse(t *testing.T) {
	idle, total := parseCPU("cpu  100 0 50 800 50 0 0 0 0 0")
	if idle != 850 || total != 1000 {
		t.Errorf("cpu %d %d", idle, total)
	}
	mt, mu, st, su := parseMem("MemTotal: 1000 kB\nMemAvailable: 250 kB\nSwapTotal: 100 kB\nSwapFree: 100 kB\n")
	if mt != 1000*1024 || mu != 750*1024 || st != 100*1024 || su != 0 {
		t.Errorf("mem %d %d %d %d", mt, mu, st, su)
	}
	iface, rx, tx := parseNet("Inter-|   Receive\n face |bytes\n    lo: 999999 1 0 0 0 0 0 0 999999 1 0 0 0 0 0 0\n  eth0: 5000 10 0 0 0 0 0 0 3000 8 0 0 0 0 0 0\n")
	if iface != "eth0" || rx != 5000 || tx != 3000 {
		t.Errorf("net %s %d %d", iface, rx, tx)
	}
	d := parseDisks("Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/sda1 1000 400 600 40% /\n")
	if len(d) != 1 || d[0].Mount != "/" || d[0].Avail != 600*1024 {
		t.Errorf("disks %+v", d)
	}
}
