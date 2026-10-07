package procs

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// parent reads ppid from /proc/<pid>/stat and argv[0] from /proc/<pid>/cmdline.
func parent(pid int) (ppid int, name string, ok bool) {
	dir := "/proc/" + strconv.Itoa(pid)
	stat, err := os.ReadFile(dir + "/stat")
	if err != nil {
		return 0, "", false
	}
	// "pid (comm) state ppid …": comm may contain spaces, so split after the last ')'.
	close := bytes.LastIndexByte(stat, ')')
	fields := strings.Fields(string(stat[close+1:]))
	if len(fields) < 2 {
		return 0, "", false
	}
	ppid, _ = strconv.Atoi(fields[1])
	if cmdline, err := os.ReadFile(dir + "/cmdline"); err == nil {
		if i := bytes.IndexByte(cmdline, 0); i > 0 {
			cmdline = cmdline[:i]
		}
		name = filepath.Base(string(cmdline))
	}
	if name == "" {
		name = string(stat[bytes.IndexByte(stat, '(')+1 : close])
	}
	return ppid, name, true
}
