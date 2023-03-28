package protocol

import "strconv"

// formatUint formats an unsigned integer to a human readable string.
func formatUint(i uint64) string {
	return strconv.FormatUint(i, 10)
}

// formatByteSize formats a byte size to a human readable string.
func formatByteSize(i uint64) string {
	return formatUint(i) + "b"
}
