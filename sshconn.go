package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// hostKeyQuestion asks the user whether to trust a host key the app has
// not seen before. The connecting goroutine waits on answer.
type hostKeyQuestion struct {
	host        string
	keyType     string
	fingerprint string
	answer      chan bool
}

// dialOptions is what dial needs besides the host.
type dialOptions struct {
	password   string
	passphrase string
	// askHostKey shows the question and returns at once; the answer comes
	// on q.answer.
	askHostKey func(q *hostKeyQuestion)
}

// knownHostsMu guards writes to the known_hosts file.
var knownHostsMu sync.Mutex

func knownHostsPath() string { return filepath.Join(configDir(), "known_hosts") }

// hostKeyCallback checks keys against the app's known_hosts file. A host
// not in it is asked about; a host whose key changed is refused.
func hostKeyCallback(opts dialOptions) (ssh.HostKeyCallback, error) {
	path := knownHostsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return nil, err
	}
	f.Close()
	check, err := knownhosts.New(path)
	if err != nil {
		return nil, err
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := check(hostname, remote, key)
		var keyErr *knownhosts.KeyError
		if err == nil || !errors.As(err, &keyErr) {
			return err
		}
		if len(keyErr.Want) > 0 {
			return fmt.Errorf("主机密钥与记录不符，可能存在中间人攻击（%s）。如果服务器确实重装过，请从 %s 中删除旧记录", ssh.FingerprintSHA256(key), path)
		}
		if opts.askHostKey == nil {
			return errors.New("未知主机密钥")
		}
		q := &hostKeyQuestion{host: hostname, keyType: key.Type(), fingerprint: ssh.FingerprintSHA256(key), answer: make(chan bool, 1)}
		opts.askHostKey(q)
		select {
		case ok := <-q.answer:
			if !ok {
				return errors.New("已拒绝该主机密钥")
			}
		case <-time.After(5 * time.Minute):
			return errors.New("等待确认主机密钥超时")
		}
		knownHostsMu.Lock()
		defer knownHostsMu.Unlock()
		out, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = fmt.Fprintln(out, knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key))
		return err
	}, nil
}

// authMethods returns how to log in to h, in the order to try them.
func authMethods(h *Host, opts dialOptions) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if h.Auth == authKey {
		data, err := os.ReadFile(expandHome(h.KeyPath))
		if err != nil {
			return nil, fmt.Errorf("读取私钥失败：%w", err)
		}
		signer, err := ssh.ParsePrivateKey(data)
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			if opts.passphrase == "" {
				return nil, errors.New("私钥有密码保护，请填写私钥密码")
			}
			signer, err = ssh.ParsePrivateKeyWithPassphrase(data, []byte(opts.passphrase))
		}
		if err != nil {
			return nil, fmt.Errorf("解析私钥失败：%w", err)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if opts.password != "" {
		pw := opts.password
		methods = append(methods, ssh.Password(pw),
			// Many servers only offer keyboard-interactive for passwords.
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = pw
				}
				return answers, nil
			}))
	}
	if len(methods) == 0 {
		return nil, errors.New("没有可用的认证方式")
	}
	return methods, nil
}

// dial connects and logs in to h.
func dial(h *Host, opts dialOptions) (*ssh.Client, error) {
	auth, err := authMethods(h, opts)
	if err != nil {
		return nil, err
	}
	hk, err := hostKeyCallback(opts)
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User:            h.User,
		Auth:            auth,
		HostKeyCallback: hk,
		Timeout:         15 * time.Second,
	}
	addr := net.JoinHostPort(h.Addr, strconv.Itoa(h.port()))
	return ssh.Dial("tcp", addr, cfg)
}

// shellConn is an interactive shell on an SSH connection, as the terminal
// view reads and writes it.
type shellConn struct {
	sess   *ssh.Session
	stdin  io.WriteCloser
	stdout io.Reader
}

func openShell(client *ssh.Client) (*shellConn, error) {
	sess, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		return nil, err
	}
	// With a pty the remote merges stderr into stdout; this catches what
	// comes before it is set up.
	sess.Stderr = sess.Stdout
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := sess.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		sess.Close()
		return nil, err
	}
	sess.Setenv("COLORTERM", "truecolor") // most servers refuse it; harmless
	if err := sess.Shell(); err != nil {
		sess.Close()
		return nil, err
	}
	return &shellConn{sess: sess, stdin: stdin, stdout: stdout}, nil
}

func (c *shellConn) Read(p []byte) (int, error)  { return c.stdout.Read(p) }
func (c *shellConn) Write(p []byte) (int, error) { return c.stdin.Write(p) }
func (c *shellConn) Close() error                { return c.sess.Close() }

// Resize is called by the terminal view as its size in cells changes.
func (c *shellConn) Resize(cols, rows int) error { return c.sess.WindowChange(rows, cols) }

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}
