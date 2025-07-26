package core

import (
	"strings"
)

// parseInputCmd parses input into command and args, respecting quoted strings.
func parseInputCmd(input string, prefix string) (cmd string, args []string) {
	inputLen := len(input)
	args = make([]string, 0, 8)
	var b strings.Builder
	inQuotes := false
	escaped := false

	// TIPS avoid rune allocation
	for i := 0; i < inputLen; i++ {
		c := input[i]

		switch {
		case escaped:
			b.WriteByte(c)
			escaped = false
		case c == '\\':
			escaped = true
		case c == '"':
			inQuotes = !inQuotes
		case c == ' ' || c == '\t':
			if inQuotes {
				b.WriteByte(c)
			} else if b.Len() > 0 {
				args = append(args, b.String())
				b.Reset()
			}
		default:
			b.WriteByte(c)
		}
	}
	if b.Len() > 0 {
		args = append(args, b.String())
	}

	if len(args) == 0 {
		return "", nil
	}
	cmd = strings.TrimPrefix(args[0], prefix)
	return cmd, args[1:]
}
