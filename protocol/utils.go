package protocol

import (
	"strconv"
	"strings"
)

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
