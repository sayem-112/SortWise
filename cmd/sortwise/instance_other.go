//go:build !windows

package main

func claimInstance(address string) (release func(), only bool) { return func() {}, true }
