//go:build !windows

package main

import "errors"

func writeCloudSession(string, string) error {
	return errors.New("secure cloud session storage is unavailable on this platform")
}

func readCloudSession(string) (string, error) {
	return "", nil
}

func clearCloudSession(string) error {
	return nil
}
