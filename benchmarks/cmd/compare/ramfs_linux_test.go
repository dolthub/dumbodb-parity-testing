package main

import (
	"syscall"
	"testing"
)

func TestRequireDiskBackedRejectsTmpfs(t *testing.T) {
	var st syscall.Statfs_t
	if err := syscall.Statfs("/dev/shm", &st); err != nil || uint32(st.Type) != tmpfsMagic {
		t.Skip("/dev/shm is not tmpfs on this host")
	}
	if err := requireDiskBacked("/dev/shm"); err == nil {
		t.Fatal("expected an error for tmpfs")
	}
}
