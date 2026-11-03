// Command kitsune-server serves a music library to Kitsune clients.
package main

import "os"

func main() {
	os.Exit(cli(os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr))
}
