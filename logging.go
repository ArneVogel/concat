package main

import (
	"fmt"
	"os"
)

func printDebug(msg ...interface{})  {
	if debug {
		fmt.Println(msg...)
	}
}

func printDebugf(format string, args ...interface{})  {
	if debug {
		fmt.Printf(format, args...)
	}
}

func printFatal(err error, msg ...interface{})  {
	if len(msg) > 0 {
		fmt.Println(msg...)
	}
	printDebug(err)
	os.Exit(1)
}

func printFatalf(err error, format string, args ...interface{})  {
	if len(format) > 0 {
		fmt.Printf(format, args...)
	}
	printDebug(err)
	os.Exit(1)
}
