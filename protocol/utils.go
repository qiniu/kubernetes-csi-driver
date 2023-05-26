package protocol

import (
	"context"
	log "github.com/sirupsen/logrus"
	"os/exec"
	"strconv"
	"strings"
)

func execOnSystemd(ctx context.Context, unit string, cmd string, args ...string) *exec.Cmd {
	finalArgs := append([]string{
		"--no-ask-password",
		"--unit=" + unit,
		"--service-type=forking",
		"--collect",
		cmd,
	})
	finalArgs = append(finalArgs, args...)
	log.Infof("Run command: %s %s", "systemd-run", strings.Join(finalArgs, " "))
	return exec.CommandContext(
		ctx,
		"systemd-run",
		finalArgs...,
	)
}

// formatUint formats an unsigned integer to a human readable string.
func formatUint(i uint64) string {
	return strconv.FormatUint(i, 10)
}

// formatByteSize formats a byte size to a human readable string.
func formatByteSize(i uint64) string {
	return formatUint(i) + "b"
}

// normalizeDirKey normalizes a directory key.
// It ensures the key not starts with a slash and ends with a slash.
func normalizeDirKey(key string) string {
	// ensure the key not starts with a slash
	if strings.HasPrefix(key, "/") {
		key = key[1:]
	}
	// ensure the key ends with a slash
	if !strings.HasSuffix(key, "/") {
		key += "/"
	}
	return key
}
