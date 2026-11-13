//go:build windows

package ms

import (
	"os"
	"syscall"
)

// renameNoReplace renames oldpath to newpath, and fails with an error that
// matches fs.ErrExist when newpath exists: MoveFile does not replace a file.
func renameNoReplace(oldpath, newpath string) error {
	from, err := syscall.UTF16PtrFromString(oldpath)
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(newpath)
	if err != nil {
		return err
	}
	if err := syscall.MoveFile(from, to); err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}
	return nil
}

// caseRenameInOneCall reports whether renameNoReplace renames between two
// spellings of one path on a case-insensitive file system.
const caseRenameInOneCall = false
