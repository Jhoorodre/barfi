package main

import "os"

func machineID() string {
	if id := os.Getenv("COMPUTERNAME"); id != "" {
		return id
	}
	h, _ := os.Hostname()
	return h
}
