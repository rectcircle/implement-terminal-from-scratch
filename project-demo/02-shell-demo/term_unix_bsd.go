// https://github.com/golang/term/blob/master/term_unix_bsd.go

//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package main

import "golang.org/x/sys/unix"

const ioctlReadTermios = unix.TIOCGETA

// const ioctlWriteTermios = unix.TIOCSETA
