package protocol

import (
	"context"
	"math/rand"
	"os/exec"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

func execOnSystemd(ctx context.Context, unit string, cmd string, args ...string) *exec.Cmd {
	finalArgs := append([]string{
		"--no-ask-password",
		"--unit=" + unit,
		"--pipe",
		"--service-type=forking",
		"--collect",
		cmd,
	}, args...)
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
	key = strings.TrimPrefix(key, "/")

	// ensure the key ends with a slash
	if !strings.HasSuffix(key, "/") {
		key += "/"
	}
	return key
}

func randomName(n int) string {
	const choices = "abcdefghijklmnopqrstuvwxyz0123456789"
	return randomChoices(choices, n)
}

func randomChoices(choices string, n int) string {
	choicesCount := len(choices)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, n)
	for i := range b {
		b[i] = choices[r.Intn(choicesCount)]
	}
	return string(b)
}
