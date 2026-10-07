package procs

import (
	"bytes"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// parent reads ppid and argv[0] via sysctl; falls back to the kernel's
// p_comm when procargs2 is denied (other user's process).
func parent(pid int) (ppid int, name string, ok bool) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, "", false
	}
	if name = argv0(pid); name == "" {
		name = unix.ByteSliceToString(kp.Proc.P_comm[:])
	}
	return int(kp.Eproc.Ppid), name, true
}

// argv0 parses kern.procargs2: int32 argc, exec path, NUL padding, argv[0], argv[1]…
func argv0(pid int) string {
	raw, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil || len(raw) <= 4 {
		return ""
	}
	rest := raw[4:]
	end := bytes.IndexByte(rest, 0)
	if end < 0 {
		return ""
	}
	rest = bytes.TrimLeft(rest[end:], "\x00")
	if end = bytes.IndexByte(rest, 0); end >= 0 {
		rest = rest[:end]
	}
	return filepath.Base(string(rest))
}
