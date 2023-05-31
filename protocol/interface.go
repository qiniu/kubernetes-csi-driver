package protocol

import (
	"context"
	"os/exec"
)

// Cmd is the interface that wraps the Command method.
type Cmd interface {
	Command()
}

// ExecutableCmd is the interface that wraps the ExecCommand method.
type ExecutableCmd interface {
	Cmd
	ExecCommand(context.Context) *exec.Cmd
}
