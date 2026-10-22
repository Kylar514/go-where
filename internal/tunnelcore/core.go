// Adapted from github.com/kylar514/diglet tunnel/state.go and tunnel/tunnel.go
// at 326775186335. MIT license retained alongside this package.
package tunnelcore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Connection struct {
	Bind                                           string
	Name, TunnelType, Resource, Namespace, SSHHost string
	LocalPort, RemotePort                          int
}
type Info struct {
	Status, PID int
	Owner       string
}
type stateEntry struct {
	Connection Connection `json:"connection"`
	PID        int        `json:"pid"`
	Identity   uint64     `json:"identity"`
}
type stateFile struct {
	Tunnels map[string]stateEntry `json:"tunnels"`
}

var builders = map[string]func(Connection) (*exec.Cmd, error){}

func Register(kind string, builder func(Connection) (*exec.Cmd, error)) { builders[kind] = builder }

func withState(fn func(*stateFile) (bool, error)) error {
	dir, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "go-where", "tunnels", "state.json")
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lock := path + ".lock"
	deadline := time.Now().Add(2 * time.Second)
	for {
		f, e := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e == nil {
			f.Close()
			break
		}
		if !errors.Is(e, os.ErrExist) || time.Now().After(deadline) {
			return fmt.Errorf("could not lock tunnel registry: %w", e)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer os.Remove(lock)
	state := stateFile{Tunnels: map[string]stateEntry{}}
	data, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(data, &state); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if state.Tunnels == nil {
		state.Tunnels = map[string]stateEntry{}
	}
	changed, err := fn(&state)
	if err != nil || !changed {
		return err
	}
	data, err = json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "state-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return replaceFile(f.Name(), path)
}

func Statuses(connections []Connection) (map[string]Info, error) {
	ports, err := listeningPorts()
	if err != nil {
		return nil, err
	}
	result := map[string]Info{}
	err = withState(func(state *stateFile) (bool, error) {
		changed := false
		for name, entry := range state.Tunnels {
			identity, e := processIdentity(entry.PID)
			if e != nil || identity != entry.Identity {
				delete(state.Tunnels, name)
				changed = true
			}
		}
		for _, c := range connections {
			entry, owned := state.Tunnels[c.Name]
			listeners := map[int]bool{}
			for endpoint, pids := range ports {
				if endpointsConflict(connectionEndpoint(c), endpoint) {
					for pid := range pids {
						listeners[pid] = true
					}
				}
			}
			listening := len(listeners) > 0
			switch {
			case owned && listening && listeners[entry.PID]:
				result[c.Name] = Info{Status: 2, PID: entry.PID}
			case owned && listening:
				result[c.Name] = Info{Status: 3, PID: entry.PID}
			case owned:
				result[c.Name] = Info{Status: 1, PID: entry.PID}
			case listening:
				info := Info{Status: 3}
				for _, owner := range state.Tunnels {
					if endpointsConflict(connectionEndpoint(c), connectionEndpoint(owner.Connection)) && listeners[owner.PID] {
						info.Owner = owner.Connection.Name
						break
					}
				}
				result[c.Name] = info
			default:
				result[c.Name] = Info{}
			}
		}
		return changed, nil
	})
	return result, err
}

func Start(c Connection) (int, error) {
	statuses, err := Statuses([]Connection{c})
	if err != nil {
		return 0, err
	}
	if statuses[c.Name].Status != 0 {
		return 0, fmt.Errorf("tunnel %q already running or port %d occupied", c.Name, c.LocalPort)
	}
	builder, ok := builders[c.TunnelType]
	if !ok {
		return 0, fmt.Errorf("unknown tunnel type %q", c.TunnelType)
	}
	cmd, err := builder(c)
	if err != nil {
		return 0, err
	}
	prepareCommand(cmd)
	if err = cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	identity, err := processIdentity(pid)
	if err != nil {
		_ = terminateProcess(pid)
		_ = cmd.Wait()
		return 0, err
	}
	err = withState(func(state *stateFile) (bool, error) {
		if _, exists := state.Tunnels[c.Name]; exists {
			return false, fmt.Errorf("tunnel already registered")
		}
		for _, entry := range state.Tunnels {
			if endpointsConflict(connectionEndpoint(entry.Connection), connectionEndpoint(c)) {
				return false, fmt.Errorf("port %d already owned", c.LocalPort)
			}
		}
		state.Tunnels[c.Name] = stateEntry{Connection: c, PID: pid, Identity: identity}
		return true, nil
	})
	if err != nil {
		_ = terminateProcess(pid)
		_ = cmd.Wait()
		return 0, err
	}
	go func() { _ = cmd.Wait() }()
	return pid, nil
}

func connectionEndpoint(c Connection) string {
	bind := c.Bind
	// Older registry entries did not store Bind; recover it from their captured argv.
	if bind == "" && c.Resource != "" {
		data, _ := base64.RawURLEncoding.DecodeString(c.Resource)
		var spec struct{ Argv []string }
		if json.Unmarshal(data, &spec) == nil {
			for i, arg := range spec.Argv {
				if i+1 >= len(spec.Argv) {
					continue
				}
				if arg == "--address" {
					bind = spec.Argv[i+1]
				}
				if arg == "-L" {
					value := spec.Argv[i+1]
					if strings.HasPrefix(value, "[") {
						if end := strings.Index(value, "]"); end > 0 {
							bind = value[1:end]
						}
					} else if end := strings.IndexByte(value, ':'); end > 0 {
						bind = value[:end]
					}
				}
			}
		}
	}
	if bind == "" {
		bind = "127.0.0.1"
	}
	return net.JoinHostPort(bind, fmt.Sprint(c.LocalPort))
}

func endpointsConflict(a, b string) bool {
	ah, ap, e := net.SplitHostPort(a)
	if e != nil {
		return true
	}
	bh, bp, e := net.SplitHostPort(b)
	if e != nil {
		return true
	}
	if ap != bp {
		return false
	}
	if ah == "*" || bh == "*" {
		return true
	}
	ai, bi := net.ParseIP(ah), net.ParseIP(bh)
	if ai == nil || bi == nil {
		return true
	}
	// Conservatively treat IPv6 wildcard as dual-stack; never allow a false-safe bind.
	if ah == "::" || bh == "::" {
		return true
	}
	if (ai.To4() != nil) != (bi.To4() != nil) {
		return false
	}
	return ai.Equal(bi) || ai.IsUnspecified() || bi.IsUnspecified()
}

func Stop(c Connection) error {
	return withState(func(state *stateFile) (bool, error) {
		entry, ok := state.Tunnels[c.Name]
		if !ok {
			return false, fmt.Errorf("no tunnel owned by go-where named %q", c.Name)
		}
		identity, err := processIdentity(entry.PID)
		if err != nil || identity != entry.Identity {
			delete(state.Tunnels, c.Name)
			return true, nil
		}
		if err = terminateProcess(entry.PID); err != nil {
			return false, err
		}
		delete(state.Tunnels, c.Name)
		return true, nil
	})
}
