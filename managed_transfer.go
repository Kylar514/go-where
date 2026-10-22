package main

import (
	"fmt"
)

const transferSnapshotMarker = "__GO_WHERE_PRIVATE_SSH_SNAPSHOT__"

func transferConfig(configs []Config) Config {
	if len(configs) > 0 {
		return configs[0]
	}
	return defaultConfig()
}

func buildTransferJob(t Transfer, h Host, sshConfig string, dry bool, c Config) (transferJob, error) {
	job := transferJob{Label: t.Mode + " " + t.Direction + " · " + h.Alias}
	if h.SourceID != "" {
		ssh, err := managedJob(h, c)
		if err != nil {
			return job, err
		}
		job.Configuration = ssh.Configuration
		h.SourceID = ""
		h.Alias = ssh.Target
		sshConfig = transferSnapshotMarker
	}
	argv, err := transferCommand(t, h, sshConfig, dry)
	if err != nil {
		return job, err
	}
	if c.debugLaunch {
		argv = verboseTransfer(argv)
	}
	job.Argv = argv
	return job, nil
}

func prepareTransferJob(job transferJob) ([]string, func(), error) {
	if len(job.Argv) == 0 || (job.Argv[0] != "scp" && job.Argv[0] != "rsync") {
		return nil, nil, fmt.Errorf("invalid transfer executable")
	}
	argv := append([]string(nil), job.Argv...)
	if job.Configuration == "" {
		return argv, func() {}, nil
	}
	path, cleanup, err := sshSnapshot(job.Configuration)
	if err != nil {
		return nil, nil, err
	}
	replaced := false
	for i := 1; i < len(argv); i++ {
		if argv[i-1] == "-F" && argv[i] == transferSnapshotMarker {
			argv[i] = path
			replaced = true
		}
		if argv[i-1] == "-e" && job.Argv[0] == "rsync" {
			// Rebuild the shell command, never substitute raw paths into shell text.
			ssh := []string{"ssh", "-F", path}
			if argv[i] == shellCommand([]string{"ssh", "-F", transferSnapshotMarker})+" -vv" {
				ssh = append(ssh, "-vv")
			} else if argv[i] != shellCommand([]string{"ssh", "-F", transferSnapshotMarker}) {
				cleanup()
				return nil, nil, fmt.Errorf("invalid managed rsync SSH command")
			}
			argv[i] = shellCommand(ssh)
			replaced = true
		}
	}
	if !replaced {
		cleanup()
		return nil, nil, fmt.Errorf("managed transfer has no snapshot argument")
	}
	return argv, cleanup, nil
}
