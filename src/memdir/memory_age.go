package memdir

import (
	"fmt"
	"time"
)

// Port of TS src/memdir/memoryAge.ts

func memoryAgeDays(mtimeMs int64) int64 {
	d := (time.Now().UnixMilli() - mtimeMs) / 86_400_000
	if d < 0 {
		return 0
	}
	return d
}

func memoryAge(mtimeMs int64) string {
	d := memoryAgeDays(mtimeMs)
	if d == 0 {
		return "today"
	}
	if d == 1 {
		return "yesterday"
	}
	return fmt.Sprintf("%d days ago", d)
}

func memoryFreshnessText(mtimeMs int64) string {
	d := memoryAgeDays(mtimeMs)
	if d <= 1 {
		return ""
	}
	return fmt.Sprintf("This memory is %d days old. Memories are point-in-time observations, not live state — claims about code behavior or file:line citations may be outdated. Verify against current code before asserting as fact.", d)
}
