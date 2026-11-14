//go:build !darwin

package ms

import "os"

// syncFile flushes f to the drive.
func syncFile(f *os.File) error { return f.Sync() }
