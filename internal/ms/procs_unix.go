//go:build !windows

package ms

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// psTimeout bounds the ps run. A ps that hangs, such as on a stuck mount,
// then counts as a process table that cannot be read.
const psTimeout = 3 * time.Second

// processTable lists every process as pid -> parent and name, from ps.
func processTable() (map[int]proc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), psTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,ppid=,comm=")
	// A child of ps that keeps the output pipe open must not hold Output.
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
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
