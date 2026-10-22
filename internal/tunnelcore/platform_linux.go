// Adapted from Diglet process_linux.go, listener_linux.go, replace_linux.go; MIT.
//go:build linux

package tunnelcore

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

func prepareCommand(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func processIdentity(pid int) (uint64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return 0, fmt.Errorf("invalid process stat")
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 20 {
		return 0, fmt.Errorf("invalid process stat")
	}
	return strconv.ParseUint(fields[19], 10, 64)
}
func terminateProcess(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err == nil {
		return nil
	}
	return syscall.Kill(pid, syscall.SIGKILL)
}
func replaceFile(from, to string) error { return os.Rename(from, to) }

var listenerPIDPattern = regexp.MustCompile(`pid=(\d+)`)

func listeningPorts() (map[string]map[int]bool, error) {
	out, err := exec.Command("ss", "-H", "-ltnp").Output()
	if err != nil {
		return nil, fmt.Errorf("inspect listeners with ss: %w", err)
	}
	ports := map[string]map[int]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		colon := strings.LastIndexByte(fields[3], ':')
		if colon < 0 {
			continue
		}
		_, err := strconv.Atoi(fields[3][colon+1:])
		if err != nil {
			continue
		}
		pid := 0
		if match := listenerPIDPattern.FindStringSubmatch(line); match != nil {
			pid, _ = strconv.Atoi(match[1])
		}
		endpoint := fields[3]
		if ports[endpoint] == nil {
			ports[endpoint] = map[int]bool{}
		}
		ports[endpoint][pid] = true
	}
	return ports, nil
}
