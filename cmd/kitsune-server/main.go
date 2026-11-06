// Command kitsune-server serves a music library to Kitsune clients.
package main

import (
	"os"
	_ "time/tzdata" // quick_scan is local time, and a scratch image has no zone files.
)

func main() {
	os.Exit(cli(os.Args[1:], os.LookupEnv, os.Stdin, os.Stdout, os.Stderr))
}
