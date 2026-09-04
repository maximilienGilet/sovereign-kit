//go:build !unix

package clientprofile

import (
	"context"
	"fmt"
)

func runPackage(context.Context, string, []string, []string) error {
	return fmt.Errorf("safe package installation is supported on Unix only")
}
