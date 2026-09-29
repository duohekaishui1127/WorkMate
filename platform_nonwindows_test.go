//go:build !windows

package main

import "errors"

// Core tests never touch a user's Windows trial anchors.
func readSecurityRegistry(string) (string, error) { return "", errors.New("registry unavailable") }
func writeSecurityRegistry(string, string) error  { return errors.New("registry unavailable") }
func readMachineGuid() (string, error)            { return "", errors.New("registry unavailable") }
