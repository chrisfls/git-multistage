//go:build !linux && !darwin && !windows

package ms

// renameNoReplace renames oldpath to newpath, and fails with an error that
// matches fs.ErrExist when newpath exists.
func renameNoReplace(oldpath, newpath string) error { return linkRename(oldpath, newpath) }
