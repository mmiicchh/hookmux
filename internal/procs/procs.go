//go:build darwin || linux

// Package procs walks the process ancestry without spawning anything.
// Names are argv[0] basenames, so a renamed or wrapped agent shows the name it was invoked by.
package procs

import "os"

const maxDepth = 64

// Ancestors returns the names of parent → … → init for the current process.
func Ancestors() []string {
	names := []string{}
	for pid := os.Getppid(); pid > 1 && len(names) < maxDepth; {
		ppid, name, ok := parent(pid)
		if !ok {
			break
		}
		names = append(names, name)
		pid = ppid
	}
	return names
}
