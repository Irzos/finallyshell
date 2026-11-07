package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type authKind string

const (
	authPassword authKind = "password"
	authKey      authKind = "key"
)

// Host is a saved connection. Passwords are not saved: they stay in
// memory for as long as the app runs.
type Host struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Addr    string   `json:"addr"`
	Port    int      `json:"port,omitempty"`
	User    string   `json:"user"`
	Auth    authKind `json:"auth"`
	KeyPath string   `json:"keyPath,omitempty"`
}

func (h *Host) port() int {
	if h.Port == 0 {
		return 22
	}
	return h.Port
}

func (h *Host) title() string {
	if h.Name != "" {
		return h.Name
	}
	return h.Addr
}

// target is user@addr, with the port when it is not 22.
func (h *Host) target() string {
	s := h.User + "@" + h.Addr
	if h.port() != 22 {
		s += ":" + strconv.Itoa(h.port())
	}
	return s
}

func newHostID() string { return fmt.Sprintf("h%d", time.Now().UnixNano()) }

func configDir() string {
	if d := os.Getenv("FINALSHELLY_DIR"); d != "" {
		return d
	}
	d, err := os.UserConfigDir()
	if err != nil {
		d = "."
	}
	return filepath.Join(d, "finalshelly")
}

func hostsPath() string { return filepath.Join(configDir(), "hosts.json") }

func loadHosts() ([]*Host, error) {
	data, err := os.ReadFile(hostsPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var hosts []*Host
	if err := json.Unmarshal(data, &hosts); err != nil {
		return nil, err
	}
	return hosts, nil
}

func saveHosts(hosts []*Host) error {
	if err := os.MkdirAll(configDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(hosts, "", "  ")
	if err != nil {
		return err
	}
	tmp := hostsPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, hostsPath())
}
