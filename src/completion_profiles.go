package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"unicode/utf16"
)

const completionMarkerStart = "# >>> my-toolbox completion >>>"
const completionMarkerEnd = "# <<< my-toolbox completion <<<"

type completionProfileRequest struct {
	path       string
	sourceLine string
	newline    string
}

type completionProfileUpdate struct {
	path     string
	original []byte
	updated  []byte
	mode     os.FileMode
}

// removeCompletionProfileBlocks removes validated managed blocks transactionally.
//
// Args:
//   - requests: Profile paths, exact source lines, and installer newline styles.
//
// Returns:
//   - error: Profile inspection, malformed-marker, write, or rollback failure.
func removeCompletionProfileBlocks(requests []completionProfileRequest) error {
	// Validate every profile before changing any profile so one malformed block
	// cannot leave another shell partially deactivated.
	updates := make([]completionProfileUpdate, 0, len(requests))
	for _, request := range requests {
		update, changed, err := prepareCompletionProfileRemoval(request)
		if err != nil {
			return err
		}
		if changed {
			updates = append(updates, update)
		}
	}
	for index, update := range updates {
		if err := os.WriteFile(update.path, update.updated, update.mode.Perm()); err != nil {
			rollbackErrors := []error{fmt.Errorf("write shell profile %s: %w", update.path, err)}
			if restoreErr := os.WriteFile(update.path, update.original, update.mode.Perm()); restoreErr != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore shell profile %s: %w", update.path, restoreErr))
			}
			for rollbackIndex := index - 1; rollbackIndex >= 0; rollbackIndex-- {
				previous := updates[rollbackIndex]
				if restoreErr := os.WriteFile(previous.path, previous.original, previous.mode.Perm()); restoreErr != nil {
					rollbackErrors = append(rollbackErrors, fmt.Errorf("restore shell profile %s: %w", previous.path, restoreErr))
				}
			}
			return errors.Join(rollbackErrors...)
		}
	}
	return nil
}

// prepareCompletionProfileRemoval validates and removes one exact managed block in memory.
//
// Args:
//   - request: Profile path, exact source line, and newline style used by its installer.
//
// Returns:
//   - completionProfileUpdate: Original and updated bytes plus the existing file mode.
//   - bool: True when the exact managed block was present.
//   - error: Profile read or malformed-marker failure.
func prepareCompletionProfileRemoval(request completionProfileRequest) (completionProfileUpdate, bool, error) {
	content, err := os.ReadFile(request.path)
	if os.IsNotExist(err) {
		return completionProfileUpdate{}, false, nil
	}
	if err != nil {
		return completionProfileUpdate{}, false, fmt.Errorf("read shell profile %s: %w", request.path, err)
	}
	info, err := os.Stat(request.path)
	if err != nil {
		return completionProfileUpdate{}, false, fmt.Errorf("inspect shell profile %s: %w", request.path, err)
	}
	startMarker := encodeCompletionProfileText(content, completionMarkerStart)
	endMarker := encodeCompletionProfileText(content, completionMarkerEnd)
	startCount := bytes.Count(content, startMarker)
	endCount := bytes.Count(content, endMarker)
	if startCount == 0 && endCount == 0 {
		return completionProfileUpdate{}, false, nil
	}
	if startCount != 1 || endCount != 1 {
		return completionProfileUpdate{}, false, fmt.Errorf("malformed my-toolbox completion markers in %s", request.path)
	}
	exactBlock := encodeCompletionProfileText(content, completionMarkerStart+request.newline+request.sourceLine+request.newline+completionMarkerEnd+request.newline)
	if bytes.Count(content, exactBlock) != 1 {
		return completionProfileUpdate{}, false, fmt.Errorf("malformed my-toolbox completion block in %s", request.path)
	}
	blockIndex := bytes.Index(content, exactBlock)
	removeIndex := blockIndex
	prefix := encodeCompletionProfileText(content, request.newline)
	if blockIndex > 0 {
		// The installer always owns one separator before a block appended to a
		// nonempty profile, so removing it restores the preceding bytes exactly.
		if blockIndex < len(prefix) || !bytes.Equal(content[blockIndex-len(prefix):blockIndex], prefix) {
			return completionProfileUpdate{}, false, fmt.Errorf("malformed my-toolbox completion block in %s", request.path)
		}
		removeIndex -= len(prefix)
	}
	updated := append([]byte(nil), content[:removeIndex]...)
	updated = append(updated, content[blockIndex+len(exactBlock):]...)
	return completionProfileUpdate{path: request.path, original: content, updated: updated, mode: info.Mode()}, true, nil
}

// encodeCompletionProfileText encodes ASCII managed text like an existing profile.
//
// Args:
//   - profile: Existing profile bytes whose byte-order mark selects UTF-16 when present.
//   - text: Managed ASCII marker, source command, or newline text to encode.
//
// Returns:
//   - []byte: UTF-16LE, UTF-16BE, or single-byte UTF-8/ANSI-compatible text bytes.
func encodeCompletionProfileText(profile []byte, text string) []byte {
	// PowerShell profiles may be UTF-16. Matching markers in the profile's
	// encoding avoids transcoding or otherwise changing unrelated bytes.
	if len(profile) < 2 || !((profile[0] == 0xff && profile[1] == 0xfe) || (profile[0] == 0xfe && profile[1] == 0xff)) {
		return []byte(text)
	}
	var order binary.ByteOrder = binary.LittleEndian
	if profile[0] == 0xfe {
		order = binary.BigEndian
	}
	units := utf16.Encode([]rune(text))
	encoded := make([]byte, len(units)*2)
	for index, unit := range units {
		order.PutUint16(encoded[index*2:], unit)
	}
	return encoded
}
