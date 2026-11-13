//go:build darwin

package ms

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

const (
	sysRenameatxNP = 488 // renameatx_np, which renamex_np calls
	atFDCWD        = -2  // AT_FDCWD
	renameExcl     = 4   // RENAME_EXCL
)

// renameNoReplace renames oldpath to newpath, and fails with an error that
// matches fs.ErrExist when newpath exists. It uses renameatx_np with
// RENAME_EXCL, and link then unlink where the file system does not support it.
func renameNoReplace(oldpath, newpath string) error {
	from, err := syscall.BytePtrFromString(oldpath)
	if err != nil {
		return err
	}
	to, err := syscall.BytePtrFromString(newpath)
	if err != nil {
		return err
	}
	cwd := atFDCWD
	//nolint:gosec // G103: the system call reads the two path strings, which stay alive during the call
	_, _, errno := syscall.Syscall6(sysRenameatxNP, uintptr(cwd), uintptr(unsafe.Pointer(from)), uintptr(cwd), uintptr(unsafe.Pointer(to)), renameExcl, 0)
	switch {
	case errno == 0:
		return nil
	case errors.Is(errno, syscall.ENOTSUP) || errors.Is(errno, syscall.EINVAL) || errors.Is(errno, syscall.ENOSYS):
		return linkRename(oldpath, newpath)
	}
	return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: errno}
}
