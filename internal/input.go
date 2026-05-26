package internal

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func readTrimmedStdinLine() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(input), nil
}

func promptText(config *CurdConfig, prompt string, allowEmpty bool) (string, error) {
	var input string
	var err error
	if config != nil && config.RofiSelection {
		input, err = GetUserInputFromRofi(prompt)
	} else {
		CurdOut(prompt)
		input, err = readTrimmedStdinLine()
	}
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)
	if input == "" && !allowEmpty {
		return "", fmt.Errorf("input cannot be empty")
	}
	return input, nil
}

func parseNonNegativeIntInput(input, label string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", label, err)
	}
	if value < 0 {
		return 0, fmt.Errorf("%s cannot be negative", label)
	}
	return value, nil
}

func parsePositiveIntInput(input, label string) (int, error) {
	value, err := parseNonNegativeIntInput(input, label)
	if err != nil {
		return 0, err
	}
	if value == 0 {
		return 0, fmt.Errorf("%s must be greater than zero", label)
	}
	return value, nil
}

func promptPositiveEpisodeNumber(config *CurdConfig, prompt string) (int, error) {
	input, err := promptText(config, prompt, false)
	if err != nil {
		return 0, err
	}
	return parsePositiveIntInput(input, "episode number")
}

func isAffirmativeAnswer(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
