package main

import (
	"flag"
	"fmt"
	"os"
)

// main parses CLI flags and delegates execution to the TUI monitor runtime.
func main() {
	host := flag.String("host", "localhost", "Server host or IP")
	port := flag.Int("port", 8080, "Server TCP port")
	flag.Parse()

	address := fmt.Sprintf("%s:%d", *host, *port)

	if err := runMonitor(address); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
}

// Código gerado com auxílio de IA.
