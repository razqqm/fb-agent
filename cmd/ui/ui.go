package ui

import (
	"fmt"
	"os"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[0;32m"
	colorYellow = "\033[1;33m"
	colorRed    = "\033[0;31m"
	colorBlue   = "\033[0;34m"
)

// OK prints a green [FB] prefixed success message.
func OK(msg string) { fmt.Printf("%s[FB]%s %s\n", colorGreen, colorReset, msg) }

// Warn prints a yellow [FB] prefixed warning message.
func Warn(msg string) { fmt.Printf("%s[FB]%s %s\n", colorYellow, colorReset, msg) }

// Err prints a red [FB] prefixed error message to stderr.
func Err(msg string) { fmt.Fprintf(os.Stderr, "%s[FB]%s %s\n", colorRed, colorReset, msg) }

// Info prints a blue [FB] prefixed informational message.
func Info(msg string) { fmt.Printf("%s[FB]%s %s\n", colorBlue, colorReset, msg) }
