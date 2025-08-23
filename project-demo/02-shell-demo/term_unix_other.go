// https://github.com/golang/term/blob/master/term_unix_other.go

//go:build aix || linux || solaris || zos

package main

import "golang.org/x/sys/unix"

const ioctlReadTermios = unix.TCGETS

// const ioctlWriteTermios = unix.TCSETS
