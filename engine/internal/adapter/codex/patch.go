package codex

import (
	"strings"

	"github.com/yerinsabraham/trackline/engine/internal/event"
)

// Codex writes files through apply_patch, which hands the hook a single command
// string rather than a structured file path:
//
//	*** Begin Patch
//	*** Add File: notes.txt
//	+hello
//	*** End Patch
//
// So "which files does this touch" is a parse here where it is a field read on
// Claude Code. This is the main thing the two hosts do not share, and isolating
// it in the adapter is what keeps the rest of the engine host-agnostic.
const (
	beginMarker  = "*** Begin Patch"
	addPrefix    = "*** Add File:"
	updatePrefix = "*** Update File:"
	deletePrefix = "*** Delete File:"
	movePrefix   = "*** Move to:"
)

type patchOp struct {
	path   string
	action event.ActionType
}

// parsePatch pulls the file operations out of an apply_patch body.
//
// ok is false when the input does not look like a patch at all. The caller must
// then mark paths unknown rather than concluding the action touched nothing: a
// patch we failed to read is not a patch that changed nothing.
func parsePatch(cmd string) (ops []patchOp, ok bool) {
	if !strings.Contains(cmd, beginMarker) {
		return nil, false
	}

	for _, line := range strings.Split(cmd, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, addPrefix):
			ops = append(ops, patchOp{path(line, addPrefix), event.ActionWriteFile})
		case strings.HasPrefix(line, updatePrefix):
			ops = append(ops, patchOp{path(line, updatePrefix), event.ActionEditFile})
		case strings.HasPrefix(line, deletePrefix):
			ops = append(ops, patchOp{path(line, deletePrefix), event.ActionDeleteFile})
		case strings.HasPrefix(line, movePrefix):
			// A rename touches the destination as well as the source, and the
			// source has already been recorded by its Update or Add line.
			ops = append(ops, patchOp{path(line, movePrefix), event.ActionWriteFile})
		}
	}

	// A patch with a begin marker but no file operations is malformed, not
	// empty. Report it as unreadable.
	if len(ops) == 0 {
		return nil, false
	}
	return ops, true
}

func path(line, prefix string) string {
	return strings.TrimSpace(strings.TrimPrefix(line, prefix))
}
