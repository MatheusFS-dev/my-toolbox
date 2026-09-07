//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func publishMacroDownload(source, destination string, replace bool) error {
	sourcePath, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationPath, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	flags := uint32(windows.MOVEFILE_WRITE_THROUGH)
	if replace {
		flags |= windows.MOVEFILE_REPLACE_EXISTING
	}
	if err := windows.MoveFileEx(sourcePath, destinationPath, flags); err != nil {
		return fmt.Errorf("publish macro download: %w", err)
	}
	return nil
}
