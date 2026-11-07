package main

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestSSH runs against a real server, named as user:password@host:port
// by GOSSH_TEST_SSH:
//
//	GOSSH_TEST_SSH=tester:secret@127.0.0.1:2222 go test -run SSH
func TestSSH(t *testing.T) {
	spec := os.Getenv("GOSSH_TEST_SSH")
	if spec == "" {
		t.Skip("GOSSH_TEST_SSH not set")
	}
	cred, addr, _ := strings.Cut(spec, "@")
	user, pass, _ := strings.Cut(cred, ":")
	hostname, portStr, _ := strings.Cut(addr, ":")
	port, _ := strconv.Atoi(portStr)

	t.Setenv("GOSSH_DIR", t.TempDir())
	posts := make(chan func(), 256)
	a := newApp(func(fn func()) { posts <- fn })
	a.saveHostsFunc = func([]*Host) error { return nil }
	// until runs what the goroutines posted, as the UI thread would, until
	// cond holds.
	until := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.After(20 * time.Second)
		for !cond() {
			select {
			case fn := <-posts:
				fn()
			case <-time.After(50 * time.Millisecond):
			case <-deadline:
				t.Fatalf("timed out waiting for %s", what)
			}
		}
	}

	h := &Host{ID: "test", Addr: hostname, Port: port, User: user, Auth: authPassword}
	a.hosts = []*Host{h}
	a.passwords[h.ID] = pass
	a.openSession(h)
	s := a.current()

	until("the host key question", func() bool { return s.hostKeyQ != nil || s.status == statusFailed })
	if s.status == statusFailed {
		t.Fatal(s.err)
	}
	if !strings.HasPrefix(s.hostKeyQ.fingerprint, "SHA256:") {
		t.Errorf("fingerprint %q", s.hostKeyQ.fingerprint)
	}
	s.hostKeyQ.answer <- true
	s.hostKeyQ = nil

	until("the shell", func() bool { return s.status == statusConnected || s.status == statusFailed })
	if s.status != statusConnected {
		t.Fatal(s.err)
	}
	s.term.Send([]byte("echo hello-$((40+2)); stty size\r"))
	until("the shell's output", func() bool { return strings.Contains(s.term.Text(), "hello-42") })

	until("the server's stats", func() bool { return s.stats.OK && s.stats.MemTotal > 0 && len(s.stats.LatHist) >= 2 })
	t.Logf("stats: cpu %.1f%% mem %s/%s load %v disks %d procs %d latency %v",
		s.stats.CPU, formatBytes(float64(s.stats.MemUsed)), formatBytes(float64(s.stats.MemTotal)), s.stats.Load, len(s.stats.Disks), len(s.stats.Procs), s.stats.Latency)

	until("the home directory", func() bool { return s.files.cwd != "" })
	names := []string{}
	for _, e := range s.files.entries {
		names = append(names, e.Name)
	}
	if !slices.Contains(names, ".bashrc") {
		t.Errorf("home %s lists %v", s.files.cwd, names)
	}
	a.openDir(s, "/")
	until("the root directory", func() bool { return s.files.cwd == "/" && !s.files.loading })
	if !slices.Contains(s.files.subdirs["/"], "/etc") {
		t.Errorf("root subdirs %v", s.files.subdirs["/"])
	}
	a.openDir(s, "/no/such/dir")
	until("the error", func() bool { return !s.files.loading })
	if s.files.err == "" || s.files.cwd != "/" {
		t.Errorf("bad dir: err %q cwd %q", s.files.err, s.files.cwd)
	}

	// The window, with the real shell.
	tt := ui.NewTester(a.view, 1280, 820)
	for range 5 {
		tt.Frame()
		time.Sleep(100 * time.Millisecond)
	}
	saveShot(t, tt, "live.png")

	// Reconnecting asks nothing: the key is known now.
	a.reconnect(s)
	until("the reconnection", func() bool { return s.status == statusConnected || s.status == statusFailed || s.hostKeyQ != nil })
	if s.status != statusConnected {
		t.Fatalf("reconnect: status %d err %q question %v", s.status, s.err, s.hostKeyQ != nil)
	}

	// A wrong password fails with a message.
	bad := &Host{ID: "bad", Addr: hostname, Port: port, User: user, Auth: authPassword}
	a.passwords[bad.ID] = "wrong"
	a.openSession(bad)
	b := a.current()
	until("the failure", func() bool { return b.status == statusFailed })
	t.Logf("wrong password: %s", b.err)

	a.closeAll()
}
