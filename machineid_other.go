//go:build !linux && !windows

package main

import "os"

func machineID() string {
	h, _ := os.Hostname()
	return h
}
