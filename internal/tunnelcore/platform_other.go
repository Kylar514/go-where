//go:build !linux && !windows

package tunnelcore

import (
	"fmt"
	"os/exec"
)

func prepareCommand(*exec.Cmd) {}
func processIdentity(int) (uint64, error) {
	return 0, fmt.Errorf("tunnel process identity unsupported on this platform")
}
func terminateProcess(int) error {
	return fmt.Errorf("tunnel termination unsupported on this platform")
}
func replaceFile(string, string) error {
	return fmt.Errorf("tunnel registry unsupported on this platform")
}
func listeningPorts() (map[string]map[int]bool, error) {
	return nil, fmt.Errorf("tunnel monitoring supported on Linux and Windows; macOS pending")
}
