//go:build !windows

package ms

import "os"

// linkRename renames oldpath to newpath with a hard link, which fails when
// newpath exists, then removes oldpath.
func linkRename(oldpath, newpath string) error {
	if err := os.Link(oldpath, newpath); err != nil {
		return err
	}
	return os.Remove(oldpath)
}
