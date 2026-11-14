package gitx

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// indexHeaderSize is the size of the header of an index file: the signature,
// the version, and the number of entries.
const indexHeaderSize = 12

// maxIndexTrees is the number of lines that the file of remembered index
// trees keeps.
const maxIndexTrees = 64

// indexTrees remembers the tree of each index by the checksum of the index
// file, in memory and in the file named by Repo.TreeCache.
type indexTrees struct {
	mu     sync.Mutex
	loaded bool
	order  []string          // checksums, newest last
	trees  map[string]string // checksum to tree
}

// IndexChecksum returns the hex of the hash at the end of the index file at
// path, or "" when the file cannot be read, is too short to be an index, or
// has no hash: with index.skipHash, Git writes zero bytes in its place. Two
// index files with the same checksum have the same content.
func (r *Repo) IndexChecksum(path string) string {
	n := len(r.EmptyTree) / 2
	if n == 0 {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.Size() < int64(indexHeaderSize+n) {
		return ""
	}
	sum := make([]byte, n)
	if _, err := f.ReadAt(sum, info.Size()-int64(n)); err != nil {
		return ""
	}
	for _, b := range sum {
		if b != 0 {
			return hex.EncodeToString(sum)
		}
	}
	return ""
}

// KnownIndexTree returns the remembered tree of the index file at path, or ""
// when its checksum has no remembered tree. It starts no `git`.
func (r *Repo) KnownIndexTree(path string) string {
	return r.knownTree(r.IndexChecksum(path))
}

// RememberIndexTree records that the index file at path, as it is now, holds
// tree. Call it only for a tree that git-multistage knows from Git's own
// output for these bytes.
func (r *Repo) RememberIndexTree(path, tree string) {
	r.rememberTree(r.IndexChecksum(path), tree)
}

func (r *Repo) knownTree(sum string) string {
	if sum == "" {
		return ""
	}
	c := &r.indexTrees
	c.mu.Lock()
	defer c.mu.Unlock()
	r.loadIndexTrees()
	return c.trees[sum]
}

func (r *Repo) rememberTree(sum, tree string) {
	if sum == "" || len(tree) != len(r.EmptyTree) {
		return
	}
	c := &r.indexTrees
	c.mu.Lock()
	defer c.mu.Unlock()
	r.loadIndexTrees()
	if c.trees[sum] == tree {
		return
	}
	c.put(sum, tree)
	r.saveIndexTrees()
}

// put keeps tree as the newest entry, and drops the oldest ones beyond the limit.
func (c *indexTrees) put(sum, tree string) {
	if _, ok := c.trees[sum]; ok {
		for i, s := range c.order {
			if s == sum {
				c.order = append(c.order[:i], c.order[i+1:]...)
				break
			}
		}
	}
	c.order = append(c.order, sum)
	c.trees[sum] = tree
	for len(c.order) > maxIndexTrees {
		delete(c.trees, c.order[0])
		c.order = c.order[1:]
	}
}

// loadIndexTrees reads the file once for each process; the caller holds the
// lock. A missing or damaged file leaves the entries that it could read.
func (r *Repo) loadIndexTrees() {
	c := &r.indexTrees
	if c.loaded {
		return
	}
	c.loaded = true
	c.trees = map[string]string{}
	if r.TreeCache == "" {
		return
	}
	data, err := os.ReadFile(r.TreeCache)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		sum, tree, ok := strings.Cut(line, " ")
		if ok && r.isObjectID(sum) && r.isObjectID(tree) {
			c.put(sum, tree)
		}
	}
}

// isObjectID reports whether s is a hex object ID of the repository's format.
func (r *Repo) isObjectID(s string) bool {
	if len(s) != len(r.EmptyTree) {
		return false
	}
	for _, ch := range s {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

// saveIndexTrees replaces the file through a temporary file and a rename,
// without a flush. The file is only a cache, so a failed write is ignored,
// and a process that writes at the same time can drop an entry.
func (r *Repo) saveIndexTrees() {
	if r.TreeCache == "" {
		return
	}
	c := &r.indexTrees
	var b strings.Builder
	for _, sum := range c.order {
		fmt.Fprintf(&b, "%s %s\n", sum, c.trees[sum])
	}
	if os.MkdirAll(filepath.Dir(r.TreeCache), 0o777) != nil {
		return
	}
	tmp := fmt.Sprintf("%s.tmp-%d", r.TreeCache, os.Getpid())
	if os.WriteFile(tmp, []byte(b.String()), 0o666) != nil || os.Rename(tmp, r.TreeCache) != nil {
		_ = os.Remove(tmp)
	}
}
