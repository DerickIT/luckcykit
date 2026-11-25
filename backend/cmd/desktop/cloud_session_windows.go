//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

func cloudSessionPath(dataDir string) string {
	return filepath.Join(dataDir, "cloud-session.dpapi")
}

func dataBlob(value []byte) (windows.DataBlob, error) {
	if len(value) == 0 {
		return windows.DataBlob{}, errors.New("cloud session is empty")
	}
	return windows.DataBlob{Size: uint32(len(value)), Data: &value[0]}, nil
}

func blobBytes(blob *windows.DataBlob) []byte {
	if blob == nil || blob.Data == nil || blob.Size == 0 {
		return nil
	}
	return unsafe.Slice(blob.Data, blob.Size)
}

func freeBlob(blob *windows.DataBlob) {
	if blob == nil || blob.Data == nil {
		return
	}
	_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(blob.Data)))
	blob.Data = nil
	blob.Size = 0
}

func writeCloudSession(dataDir, payload string) error {
	input, err := dataBlob([]byte(payload))
	if err != nil {
		return err
	}
	var protected windows.DataBlob
	if err := windows.CryptProtectData(&input, nil, nil, 0, nil, 0, &protected); err != nil {
		return err
	}
	defer freeBlob(&protected)
	return os.WriteFile(cloudSessionPath(dataDir), blobBytes(&protected), 0o600)
}

func readCloudSession(dataDir string) (string, error) {
	protected, err := os.ReadFile(cloudSessionPath(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	input, err := dataBlob(protected)
	if err != nil {
		return "", err
	}
	var plain windows.DataBlob
	var description *uint16
	if err := windows.CryptUnprotectData(&input, &description, nil, 0, nil, 0, &plain); err != nil {
		return "", err
	}
	defer freeBlob(&plain)
	if description != nil {
		_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(description)))
	}
	return string(blobBytes(&plain)), nil
}

func clearCloudSession(dataDir string) error {
	err := os.Remove(cloudSessionPath(dataDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
