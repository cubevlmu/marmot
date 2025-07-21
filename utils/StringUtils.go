package utils

import "strings"

func ParseInputCmd(input string, prefix string) (cmd string, args []string) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", nil
	}
	cmd = strings.TrimPrefix(parts[0], prefix)
	args = parts[1:]
	return
}
