package main

import (
	"fmt"
	"strings"
)

type transferEndpoints struct {
	Mode                        string
	Source, Destination         *ProfileReference
	SourcePath, DestinationPath string
	Preview                     bool
}

func endpointTransferJob(t transferEndpoints, c Config) (transferJob, error) {
	if t.Mode != "scp" && t.Mode != "legacy-scp" && t.Mode != "rsync" {
		return transferJob{}, fmt.Errorf("invalid transfer method")
	}
	if t.Source == nil && t.Destination == nil {
		return transferJob{}, fmt.Errorf("select at least one SSH host")
	}
	if t.Source != nil && t.Destination != nil {
		if t.Mode == "rsync" {
			return transferJob{}, fmt.Errorf("remote-to-remote Rsync is not supported yet; use SCP or Legacy SCP")
		}
		if err := validateTransferPath(t.SourcePath); err != nil {
			return transferJob{}, err
		}
		if err := validateTransferPath(t.DestinationPath); err != nil {
			return transferJob{}, err
		}
		a, err := defaultTunnelHost(c, *t.Source)
		if err != nil {
			return transferJob{}, err
		}
		b, err := defaultTunnelHost(c, *t.Destination)
		if err != nil {
			return transferJob{}, err
		}
		x, err := managedJob(a, c)
		if err != nil {
			return transferJob{}, err
		}
		y, err := managedJob(b, c)
		if err != nil {
			return transferJob{}, err
		}
		// Namespace the generated aliases, including every ProxyJump reference,
		// so both endpoint chains coexist without sharing identity settings.
		namespace := func(text, prefix string) string {
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				if strings.HasPrefix(line, "Host gw-") {
					lines[i] = strings.Replace(line, "Host gw-", "Host "+prefix+"gw-", 1)
				} else if strings.HasPrefix(strings.TrimSpace(line), "ProxyJump ") {
					lines[i] = strings.ReplaceAll(line, "gw-", prefix+"gw-")
				}
			}
			return strings.Join(lines, "\n")
		}
		xtext := namespace(x.Configuration, "src-")
		ytext := namespace(y.Configuration, "dst-")
		from, to := t.SourcePath, t.DestinationPath
		argv := []string{"scp", "-r", "-3"}
		if t.Mode == "legacy-scp" {
			argv = append(argv, "-O")
			from = legacyRemotePath(from)
			to = legacyRemotePath(to)
		} else {
			argv = append(argv, "-s")
		}
		argv = append(argv, "-F", transferSnapshotMarker, "--", "src-"+x.Target+":"+from, "dst-"+y.Target+":"+to)
		if c.debugLaunch {
			argv = verboseTransfer(argv)
		}
		return transferJob{Label: t.Mode + " · " + a.Alias + " → " + b.Alias, Configuration: xtext + "\n" + ytext, Argv: argv}, nil
	}
	ref := t.Destination
	direction := "upload"
	local, remote := t.SourcePath, t.DestinationPath
	if t.Source != nil {
		ref = t.Source
		direction = "download"
		local, remote = t.DestinationPath, t.SourcePath
	}
	h, err := defaultTunnelHost(c, *ref)
	if err != nil {
		return transferJob{}, err
	}
	return buildTransferJob(Transfer{Mode: t.Mode, Direction: direction, Local: local, Remote: remote}, h, "", t.Preview && t.Mode == "rsync", c)
}

func chooseTransferEndpoint(c Config, current *ProfileReference) (*ProfileReference, bool, error) {
	options := []choice{{"local", "This computer"}}
	refs := map[string]ProfileReference{}
	selected := "local"
	if current != nil {
		selected = current.Source + ":" + current.ID
	}
	for _, s := range c.Sources {
		if !sourceEnabled(c, s.ID) {
			continue
		}
		hosts, err := sourceInventoryHosts(s)
		if err != nil {
			return nil, false, err
		}
		for _, h := range hosts {
			ref := ProfileReference{s.ID, h.ID}
			if _, err = defaultTunnelHost(c, ref); err != nil {
				continue
			}
			key := s.ID + ":" + h.ID
			refs[key] = ref
			options = append(options, choice{key, h.Alias + " — " + s.Name})
		}
	}
	id, back, err := chooseOne("Transfer host", options, "This computer or a saved host's default SSH connection and your local identity.", selected)
	if err != nil || back {
		return nil, back, err
	}
	if id == "local" {
		return nil, false, nil
	}
	ref := refs[id]
	return &ref, false, nil
}

var transferMethods = []choice{{"scp", "SCP"}, {"legacy-scp", "Legacy SCP"}, {"rsync", "Rsync"}}
var transferMethodDetails = map[string]string{
	"scp":        "SCP\n\nCopies files and directories using SFTP over SSH. Supports local/remote copies and remote-to-remote relay through this computer. Existing destination files may be overwritten. Preview only; no true dry run.",
	"legacy-scp": "Legacy SCP\n\nUses the older SCP protocol. Requires remote scp and compatible shells. Supports remote-to-remote relay, with destination authentication limitations. Existing files may be overwritten. Preview only; no true dry run.",
	"rsync":      "Rsync\n\nCopies incrementally with timestamps and supports a real dry run. Requires rsync on both ends. No deletion is requested. Remote-to-remote transfers are not supported yet. Existing destination files may be overwritten.",
}

func transferMenu(c Config, o Origin, exe string, selected ...string) error {
	previousSave, previousState := saveActiveSettings, settingsSaveState
	saveActiveSettings, settingsSaveState = nil, nil
	defer func() { saveActiveSettings, settingsSaveState = previousSave, previousState }()
	t := transferEndpoints{Mode: "scp"}
	methods := []choice{{"scp", "SCP"}, {"legacy-scp", "Legacy SCP"}, {"rsync", "Rsync"}}
	for {
		method := ""
		back := false
		var err error
		if len(selected) > 0 {
			method = selected[0]
		} else {
			m := newDialog("Transfers", methods, "")
			m.optionDetails = transferMethodDetails
			method, back, err = runChoice(m, menuKey(m.title))
		}
		if err != nil || back {
			return err
		}
		t.Mode = method
		for {
			label := func(ref *ProfileReference) string {
				if ref == nil {
					return "This computer"
				}
				h, err := defaultTunnelHost(c, *ref)
				if err != nil {
					return "Unavailable"
				}
				return h.Alias + " — " + h.SourceName
			}
			previewLabel := "Preview only"
			if t.Mode == "rsync" {
				previewLabel = "Dry run"
			}
			options := []choice{{"method", "Transfer method: " + startupTabLabel(t.Mode)}, {"source", "Source host: " + label(t.Source)}, {"source-path", "Source location: " + t.SourcePath}, {"destination", "Destination host: " + label(t.Destination)}, {"destination-path", "Destination location: " + t.DestinationPath}, {"preview", previewLabel + ": " + boolSettingLabel(&t.Preview)}}
			m := newDialog("Transfer", options, "")
			m.footerActions = []choice{{"review", "Review transfer"}}
			m.optionDetails = map[string]string{
				"method":           "Choose SCP (SFTP), Legacy SCP (remote scp and compatible shell), or Rsync. Existing destination files may be overwritten. Rsync does not delete files.",
				"source":           "Where the files come from. Select This computer or a saved SSH host. Its default SSH connection and your identity are used.",
				"destination":      "Where files are copied to. Select This computer or a saved SSH host. Remote-to-remote SCP relays data through this computer.",
				"source-path":      "File or directory on the selected source host. Local sources must exist. Remote paths are interpreted by that remote host.",
				"destination-path": "Path on the destination host. Existing files may be overwritten. Directory and trailing-slash semantics depend on the selected method.",
				"preview":          "SCP preview displays the operation without running it. Rsync dry run actually contacts the host and reports proposed changes without copying files. Neither authorizes a later real transfer.",
			}
			for _, option := range options {
				_, value, _ := strings.Cut(option.Label, ": ")
				m.optionDetails[option.ID] += "\n\nCurrent: " + value
			}
			id, back, err := runChoice(m, menuKey(m.title))
			if err != nil {
				return err
			}
			if back {
				break
			}
			switch id {
			case "method":
				value, b, e := chooseOne("Transfer method", methods, "Changing method preserves the endpoints and paths.", t.Mode)
				if e != nil {
					return e
				}
				if !b {
					t.Mode = value
				}
			case "source", "destination":
				ref := t.Source
				if id == "destination" {
					ref = t.Destination
				}
				value, b, e := chooseTransferEndpoint(c, ref)
				if e != nil {
					return e
				}
				if !b {
					if id == "source" {
						t.Source = value
					} else {
						t.Destination = value
					}
				}
			case "source-path", "destination-path":
				value := t.SourcePath
				local := t.Source == nil
				if id == "destination-path" {
					value = t.DestinationPath
					local = t.Destination == nil
				}
				value, b, e := pathPrompt("Transfer location", value, local, exe)
				if e != nil {
					return e
				}
				if !b {
					if id == "source-path" {
						t.SourcePath = value
					} else {
						t.DestinationPath = value
					}
				}
			case "preview":
				t.Preview = !t.Preview
			case "review":
				job, e := endpointTransferJob(t, c)
				if e != nil {
					if e = notice(e.Error()); e != nil {
						return e
					}
					continue
				}
				text := fmt.Sprintf("FROM %s: %s\nTO %s: %s\n\n%s\n\nExisting destination files may be overwritten. No deletion is requested.\n%s", label(t.Source), t.SourcePath, label(t.Destination), t.DestinationPath, shellCommand(job.Argv), job.Configuration)
				if t.Preview && t.Mode != "rsync" {
					if e = notice("PREVIEW ONLY — no transfer will run.\n\n" + text); e != nil {
						return e
					}
					continue
				}
				if e = checkPlacement(1, destination(c.Destination, o, c), o); e != nil {
					if e = notice(e.Error()); e != nil {
						return e
					}
					continue
				}
				action := "Run transfer — confirm writes"
				if t.Preview {
					action = "Run Rsync dry run — no copy"
				}
				_, b, e := choose("Review transfer", []choice{{"run", action}}, text)
				if e != nil {
					return e
				}
				if b {
					continue
				}
				if e = placeJobs([]string{job.Label}, [][]string{{exe, "__run-transfer", encodeJob(job)}}, destination(c.Destination, o, c), o, c); e != nil {
					return e
				}
			}
		}
		if len(selected) > 0 {
			return nil
		}
	}
}
