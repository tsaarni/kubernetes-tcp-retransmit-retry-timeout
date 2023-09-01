package main

import (
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {

	var (
		ignoreSigterm  bool
		serverHostname string
		serverPort     int
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <server|client>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.BoolVar(&ignoreSigterm, "catch-sigterm", false, "Catch and ignore SIGTERM")
	flag.StringVar(&serverHostname, "server", "server", "Hostname to connect to in client mode")
	flag.IntVar(&serverPort, "port", 8000, "Port number to listen (server) or connect (client)")

	flag.Parse()

	if ignoreSigterm {
		slog.Info("Setting signal handler to ignore SIGTERM")

		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM)
		go handleSignals(c)
	}

	switch flag.Arg(0) {
	case "server":
		server(serverPort)

	case "client":
		client(serverHostname, serverPort)

	default:
		slog.Error("No arguments given. Use 'echo server' or 'echo client'")
		os.Exit(1)
	}

}

func handleSignals(sigs chan os.Signal) {
	for {
		sig := <-sigs
		slog.Info("Ignoring received signal", "signal", sig)
	}
}

func server(serverPort int) {
	address := ":" + fmt.Sprintf("%d", serverPort)
	slog.Info("Server started", "address", address)

	socket, err := net.Listen("tcp", address)
	if err != nil {
		slog.Error("Error creating socket", "error", err)
		os.Exit(1)
	}

	for {
		conn, err := socket.Accept()
		if err != nil {
			slog.Error("Error accepting connection", "error", err)
			os.Exit(1)
		}
		go handleClientConnection(conn)
	}
}

func handleClientConnection(conn net.Conn) {
	defer conn.Close()

	slog.Info("Connection received", "remote_addr", conn.RemoteAddr().String())

	for {
		data := make([]byte, 1024)
		_, err := conn.Read(data)
		if err != nil {
			slog.Error("Error reading data", "error", err)
			break
		}
		slog.Info("Received request", "remote_addr", conn.RemoteAddr().String())

		slog.Info("Sending response", "remote_addr", conn.RemoteAddr().String())
		_, err = conn.Write(data)
		if err != nil {
			slog.Error("Error writing data", "error", err)
			break
		}
	}
	slog.Info("Connection closed", "address", conn.RemoteAddr().String())
}

func client(serverHostname string, serverPort int) {
	slog.Info("Client started")

	address := serverHostname + ":" + fmt.Sprintf("%d", serverPort)

	for {
		time.Sleep(5 * time.Second)

		slog.Info("Connecting to server", "remote_addr", address)
		conn, err := net.Dial("tcp", address)
		if err != nil {
			slog.Error("Error connecting to server", "error", err)
			continue
		}

		handleServerConnection(conn)

		slog.Info("Connection closed", "address", conn.RemoteAddr().String())
		conn.Close()
	}
}

func handleServerConnection(conn net.Conn) {
	payload := []byte("ping")

	for {
		slog.Info("Sending request", "remote_addr", conn.RemoteAddr().String(), "local_addr", conn.LocalAddr().String())
		_, err := conn.Write(payload)
		if err != nil {
			slog.Error("Error writing data", "error", err)
			break
		}

		data := make([]byte, 1024)
		_, err = conn.Read(data)
		if err != nil {
			slog.Error("Error reading data", "error", err)
			break
		}

		slog.Info("Received response", "remote_addr", conn.RemoteAddr().String(), "local_addr", conn.LocalAddr().String())

		time.Sleep(5 * time.Second)
	}
}
