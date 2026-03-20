package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

func prompt(reader *bufio.Reader, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("  %s: ", label)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return defaultVal
	}
	return line
}

func promptSecret(reader *bufio.Reader, label string) string {
	fmt.Printf("  %s: ", label)

	if term.IsTerminal(int(os.Stdin.Fd())) {
		line, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err == nil {
			return strings.TrimSpace(string(line))
		}
		fmt.Println("  (warning: failed to hide input; falling back to visible input)")
	}

	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

func withDefault(val, fallback string) string {
	if val != "" {
		return val
	}
	return fallback
}
