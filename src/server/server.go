package server

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func StartHTTPServer(port int) *http.Server {
	fmt.Println("Starting HTTP server on port", port)
	server := &http.Server{
		Addr:    ":" + strconv.Itoa(port),
		Handler: http.FileServer(http.Dir(".")),
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Error starting HTTP server: %v", err)
		}
	}()

	return server
}

func WaitForInstallation(ctx context.Context, conn net.Conn) error {
	fmt.Println("Waiting for the installation to complete...")

	buf := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			_, err := conn.Read(buf)
			if err != nil {
				if err == io.EOF || strings.Contains(err.Error(), "connection reset by peer") {
					fmt.Println("Installation completed. Connection closed by target device.")
					return nil
				}
				return fmt.Errorf("reading from connection: %w", err)
			}
		}
	}
}

func ShutdownHTTPServer(httpServer *http.Server) {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
}
