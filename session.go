package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type sessionStatus int

const (
	statusConnecting sessionStatus = iota
	statusConnected
	statusFailed
	statusClosed
)

var nextSessionID atomic.Int64

// Session is one open tab: a connection to a host, its shell, the
// server's stats and its files. The UI thread owns every field; the
// goroutines that connect and poll change them through App.post.
type Session struct {
	ID     int64
	Host   *Host
	status sessionStatus
	err    string
	ip     string
	title  string // what the shell set with OSC 0/2

	client *ssh.Client
	term   *terminal.Terminal
	stats  Stats
	sftp   *sftp.Client
	files  fileState

	hostKeyQ *hostKeyQuestion
	cancel   context.CancelFunc
}

func newSession(h *Host) *Session {
	s := &Session{ID: nextSessionID.Add(1), Host: h}
	s.files.init()
	return s
}

func (s *Session) label() string {
	if s.Host.Name != "" {
		return s.Host.Name
	}
	return s.Host.Addr
}

// connect starts connecting s on a goroutine.
func (a *App) connect(s *Session, opts dialOptions) {
	if s.cancel != nil {
		s.cancel()
	}
	s.status, s.err, s.hostKeyQ = statusConnecting, "", nil
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	opts.askHostKey = func(q *hostKeyQuestion) { a.post(func() { s.hostKeyQ = q }) }
	h := *s.Host
	go func() {
		client, err := dial(&h, opts)
		if err != nil {
			a.post(func() { s.status, s.err = statusFailed, dialError(err) })
			return
		}
		if ctx.Err() != nil { // closed while connecting
			client.Close()
			return
		}
		shell, err := openShell(client)
		if err != nil {
			client.Close()
			a.post(func() { s.status, s.err = statusFailed, "打开终端失败："+err.Error() })
			return
		}
		term, err := terminal.New(terminal.Options{
			Conn:      shell,
			Font:      a.termFont,
			Theme:     a.termTheme,
			DarkTheme: a.termTheme,
			OnTitle:   func(t string) { a.post(func() { s.title = t }) },
			OnExit: func(int) {
				a.post(func() {
					if s.status == statusConnected {
						s.status, s.err = statusClosed, "连接已断开"
					}
				})
			},
		})
		if err != nil {
			shell.Close()
			client.Close()
			a.post(func() { s.status, s.err = statusFailed, "终端初始化失败："+err.Error() })
			return
		}
		ip := h.Addr
		if addr, ok := client.RemoteAddr().(*net.TCPAddr); ok {
			ip = addr.IP.String()
		}
		a.post(func() {
			if ctx.Err() != nil {
				term.Close()
				client.Close()
				return
			}
			s.client, s.term, s.ip = client, term, ip
			s.status, s.err = statusConnected, ""
		})
		go monitor(ctx, client, 2*time.Second, func(st Stats) {
			a.post(func() {
				if ctx.Err() == nil {
					s.stats = st
				}
			})
		})
		if sc, err := sftp.NewClient(client); err == nil {
			home, _ := sc.Getwd()
			if home == "" {
				home = "/"
			}
			a.post(func() {
				if ctx.Err() != nil {
					sc.Close()
					return
				}
				s.sftp = sc
				a.openDir(s, home)
			})
		} else {
			a.post(func() { s.files.err = fmt.Sprintf("SFTP 不可用：%v", err) })
		}
	}()
}

// disconnect ends s's connection, keeping its tab.
func (s *Session) disconnect() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.hostKeyQ != nil {
		s.hostKeyQ.answer <- false
		s.hostKeyQ = nil
	}
	if s.sftp != nil {
		s.sftp.Close()
		s.sftp = nil
	}
	if s.term != nil {
		s.term.Close()
		s.term = nil
	}
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
}

func (s *Session) statusColor(t *ui.Theme) ui.Color {
	switch s.status {
	case statusConnected:
		return t.Success
	case statusConnecting:
		return t.Warning
	case statusFailed:
		return t.Danger
	}
	return t.TextMuted
}

// dialError says in the user's words why a connection failed.
func dialError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unable to authenticate"):
		return "认证失败：用户名、密码或私钥不正确"
	case strings.Contains(msg, "connection refused"):
		return "连接被拒绝：端口没有开放 SSH 服务"
	case strings.Contains(msg, "i/o timeout"):
		return "连接超时：请检查地址、端口和网络"
	case strings.Contains(msg, "no such host"):
		return "找不到主机：域名无法解析"
	}
	return msg
}
