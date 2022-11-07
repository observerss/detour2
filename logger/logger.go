package logger

import (
	"log"
	"os"
)

var (
	Flags = log.LstdFlags | log.Lmicroseconds | log.Lshortfile | log.Lmsgprefix
	Debug = log.New(os.Stdout, "DEBUG ", Flags)
	Info  = log.New(os.Stdout, "INFO  ", Flags)
	Warn  = log.New(os.Stdout, "WARN  ", Flags)
	Error = log.New(os.Stderr, "ERROR ", Flags)
)
