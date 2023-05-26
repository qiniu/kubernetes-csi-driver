package protocol

import (
	"context"
	"fmt"
	"os/exec"
)

type InitKodoFSMountCmd struct {
	VolumeId     string `json:"volume_id"`
	GatewayID    string `json:"gateway_id"`
	MountPath    string `json:"mount_path"`
	SubDir       string `json:"sub_dir"`
	RunOnSystemd bool   `json:"run_on_systemd"`
}

func (*InitKodoFSMountCmd) Command() {}

func (c *InitKodoFSMountCmd) ExecCommand(ctx context.Context) *exec.Cmd {
	var args = []string{"mount", c.GatewayID, c.MountPath, "-s", c.SubDir, "--force_reinit"}
	if c.RunOnSystemd {
		return execOnSystemd(ctx, fmt.Sprintf("run-kodofs-%s.service", c.VolumeId), KodoFSCmd, args...)
	} else {
		return exec.CommandContext(ctx, KodoFSCmd, args...)
	}
}
