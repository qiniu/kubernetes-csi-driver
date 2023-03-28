package protocol

import (
	"context"
	"os/exec"
)

type InitKodoFSMountCmd struct {
	GatewayID string `json:"gateway_id"`
	MountPath string `json:"mount_path"`
	SubDir    string `json:"sub_dir"`
}

func (*InitKodoFSMountCmd) Command() {}

func (c *InitKodoFSMountCmd) ExecCommand(ctx context.Context) *exec.Cmd {
	var args = []string{"mount", c.GatewayID, c.MountPath, "-s", c.SubDir, "--force_reinit"}
	return exec.CommandContext(ctx, KodoFSCmd, args...)
}
