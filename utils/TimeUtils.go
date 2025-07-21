package utils

import "fmt"

func FormatDuration(seconds int64) string {
	days := seconds / (24 * 3600)
	seconds %= 24 * 3600

	hours := seconds / 3600
	seconds %= 3600

	minutes := seconds / 60
	seconds %= 60

	var result string

	if days > 0 {
		result += fmt.Sprintf("%d 天 ", days)
	}
	if hours > 0 {
		result += fmt.Sprintf("%d 时 ", hours)
	}
	if minutes > 0 {
		result += fmt.Sprintf("%d 分 ", minutes)
	}
	if seconds > 0 || result == "" {
		result += fmt.Sprintf("%d 秒", seconds)
	}

	return result
}
