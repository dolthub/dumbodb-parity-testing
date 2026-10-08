//go:build !linux

package main

func requireDiskBacked(dir string) error {
	return nil
}
