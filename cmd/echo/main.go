package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Simple test program that writes to stdout/stderr 'n' number of times every
// 500 millisconds. 'n' is provided as the only command line argument
func main() {
	if len(os.Args) != 2 {
		os.Exit(-1)
	}

	repeatCount := os.Args[1]
	cnt, err := strconv.Atoi(repeatCount)
	if err != nil {
		panic(err)
	}

	for idx := range cnt {
		fmt.Printf("stdout %d\n", idx+1)
		fmt.Fprintf(os.Stderr, "stderr %d\n", idx+1)
		time.Sleep(500 * time.Millisecond)
	}
}
