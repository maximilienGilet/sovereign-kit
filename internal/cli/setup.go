// Package cli implements the small interactive Sovereign Kit setup flow.
package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
)

// Setup creates the only V1 configuration: the built-in qwen-studio SSH route.
func Setup(input io.Reader, output io.Writer, configPath, defaultUser string) error {
	reader := bufio.NewReader(input)
	fmt.Fprintln(output, "Sovereign Kit setup")
	fmt.Fprintln(output, "Profile: qwen-studio (private Qwen through SSH loopback)")

	host, err := ask(reader, output, "GPU host", "")
	if err != nil {
		return err
	}
	portText, err := ask(reader, output, "SSH port", "22")
	if err != nil {
		return err
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		return fmt.Errorf("SSH port must be a number")
	}
	user, err := ask(reader, output, "SSH user", defaultUser)
	if err != nil {
		return err
	}
	identity, err := ask(reader, output, "SSH identity file", "")
	if err != nil {
		return err
	}
	knownHosts, err := ask(reader, output, "Verified known-hosts file", "")
	if err != nil {
		return err
	}
	for label, path := range map[string]string{"SSH identity file": identity, "verified known-hosts file": knownHosts} {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return fmt.Errorf("%s is not a readable file: %s", label, path)
		}
	}

	if err := config.Save(configPath, config.Studio(host, port, user, identity, knownHosts)); err != nil {
		return err
	}
	fmt.Fprintf(output, "Configuration saved: %s\n", configPath)
	fmt.Fprintln(output, "Next: sovkit tunnel, then sovkit doctor.")
	return nil
}

func ask(reader *bufio.Reader, output io.Writer, label, defaultValue string) (string, error) {
	if defaultValue == "" {
		fmt.Fprintf(output, "%s: ", label)
	} else {
		fmt.Fprintf(output, "%s [%s]: ", label, defaultValue)
	}
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		value = defaultValue
	}
	if value == "" {
		return "", fmt.Errorf("%s is required", strings.ToLower(label))
	}
	return value, nil
}
