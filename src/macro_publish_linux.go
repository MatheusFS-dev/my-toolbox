//go:build linux

package main

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func publishMacroDownload(source, destination string, replace bool) error {
	if replace {
		return os.Rename(source, destination)
	}
	if err := unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE); err != nil {
		return fmt.Errorf("publish macro without overwriting: %w", err)
	}
	return nil
}
