//go:build !windows

package ms

import (
	"os/exec"
	"strconv"
	"strings"
)

// processTable lists every process as pid -> parent and name, from ps.
func processTable() (map[int]proc, error) {
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=,comm=").Output()
	if err != nil {
		return nil, err
	}
	table := map[int]proc{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		table[pid] = proc{pid: pid, ppid: ppid, name: procName(strings.Join(f[2:], " "))}
	}
	return table, nil
}
