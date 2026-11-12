//go:build windows

package ms

import (
	"syscall"
	"unsafe"
)

// processTable lists every process as pid -> parent and name, from a
// Toolhelp snapshot.
func processTable() (map[int]proc, error) {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(snap)
	table := map[int]proc{}
	var e syscall.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = syscall.Process32First(snap, &e); err == nil; err = syscall.Process32Next(snap, &e) {
		table[int(e.ProcessID)] = proc{pid: int(e.ProcessID), ppid: int(e.ParentProcessID), name: procName(syscall.UTF16ToString(e.ExeFile[:]))}
	}
	return table, nil
}
