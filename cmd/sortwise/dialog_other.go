//go:build !windows

package main

import "fmt"

func showError(title, message string) {
	fmt.Printf("%s: %s\n", title, message)
}
