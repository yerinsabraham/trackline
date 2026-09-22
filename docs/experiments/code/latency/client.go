package main

import (
	"bufio"
	"io"
	"net"
	"os"
	"strings"
)

func main() {
	raw, _ := io.ReadAll(os.Stdin)
	conn, err := net.Dial("unix", "/tmp/trackline-bench.sock")
	if err != nil {
		os.Exit(0) // daemon down: fail open, never break the session
	}
	defer conn.Close()
	conn.Write(append(raw, '\n'))
	resp, _ := bufio.NewReader(conn).ReadString('\n')
	if strings.HasPrefix(resp, "2 ") {
		os.Stderr.WriteString(strings.TrimSpace(resp[2:]))
		os.Exit(2)
	}
	os.Exit(0)
}
