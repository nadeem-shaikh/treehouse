package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// stdinReader is shared across Confirm calls so that one prompt's buffered
// read-ahead is not lost when a later prompt runs in the same invocation.
var stdinReader = bufio.NewReader(os.Stdin)

// IsInteractive reports whether standard input is connected to a terminal, so
// callers can decide whether prompting the user for confirmation makes sense.
func IsInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func Confirm(message string, defaultYes bool) (bool, error) {
	hint := "Y/n"
	if !defaultYes {
		hint = "y/N"
	}

	fmt.Fprintf(os.Stderr, "%s [%s] ", message, hint)

	input, err := stdinReader.ReadString('\n')
	if err != nil {
		return defaultYes, err
	}

	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return defaultYes, nil
	}

	return input == "y" || input == "yes", nil
}
