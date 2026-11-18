//go:build !windows

package main

import "os"

func openTranscript(path string) (*os.File, error) { return os.Open(path) }
