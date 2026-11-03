package engine

import (
	"strconv"
	"strings"
)

// parseRcloneStats extracts the number of bytes rclone reports as transferred.
// It understands the --stats-one-line format:
//
//	Transferred:   1.234 GiB / 2.000 GiB, 61%, 10.1 MiB/s, ETA 1m2s
//	Transferred:   2.41 GBytes, 41.8 MBytes/s, ETA 12s
//
// The confirmed byte count is taken from this report rather than from a local
// send counter, so the green progress segment only grows once the cloud has
// accepted the data (back_spec.md §5.2).
func parseRcloneStats(line string) (int64, bool) {
	idx := strings.Index(line, "Transferred:")
	if idx < 0 {
		return 0, false
	}
	rest := strings.TrimSpace(line[idx+len("Transferred:"):])
	fields := strings.Fields(rest)
	if len(fields) < 2 {
		return 0, false
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, false
	}
	unit := strings.TrimRight(fields[1], ",/;")
	return int64(value * unitMultiplier(unit)), true
}

func unitMultiplier(unit string) float64 {
	switch strings.ToLower(unit) {
	case "b", "byte", "bytes":
		return 1
	case "kib", "kbytes", "kb":
		return 1024
	case "mib", "mbytes", "mb":
		return 1024 * 1024
	case "gib", "gbytes", "gb":
		return 1024 * 1024 * 1024
	case "tib", "tbytes", "tb":
		return 1024 * 1024 * 1024 * 1024
	case "pib", "pbytes", "pb":
		return 1024 * 1024 * 1024 * 1024 * 1024
	default:
		return 0
	}
}
