//go:build !windows

package main

import "os"

func replaceAPICache(from, to string) error { return os.Rename(from, to) }
