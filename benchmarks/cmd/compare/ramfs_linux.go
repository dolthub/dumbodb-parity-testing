package main

import (
	"fmt"
	"syscall"
)

const (
	tmpfsMagic = 0x01021994
	ramfsMagic = 0x858458f6
)

func requireDiskBacked(dir string) error {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return fmt.Errorf("statfs %s: %w", dir, err)
	}
	switch uint32(st.Type) {
	case tmpfsMagic, ramfsMagic:
		return fmt.Errorf("%s is on a RAM filesystem, so syncs never reach disk; set -work-dir to a disk-backed directory", dir)
	}
	return nil
}
