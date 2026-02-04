package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type ConfirmationResult int

const (
	ConfirmYes ConfirmationResult = iota
	ConfirmNo
	ConfirmAll
)

// AskConfirmation prompts the user with a message and expects y/n/a.
// Returns the result.
func AskConfirmation(msg string) ConfirmationResult {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s [y/n/a]: ", msg)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "y", "yes":
			return ConfirmYes
		case "n", "no":
			return ConfirmNo
		case "a", "all":
			return ConfirmAll
		}
	}
}
