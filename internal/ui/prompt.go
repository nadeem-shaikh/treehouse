package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
)

// stdinReader is shared across Confirm calls so that one prompt's buffered
// read-ahead is not lost when a later prompt runs in the same invocation.
var stdinReader = bufio.NewReader(os.Stdin)

// IsInteractive reports whether standard input is connected to a terminal, so
// callers can decide whether prompting the user for confirmation makes sense.
// It uses a real terminal check rather than a character-device test, so
// non-interactive stdin such as /dev/null (itself a character device) is
// correctly treated as non-interactive.
func IsInteractive() bool {
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
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
