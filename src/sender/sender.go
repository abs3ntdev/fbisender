package sender

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"git.asdf.cafe/abs3nt/fbisender/src/config"
	"git.asdf.cafe/abs3nt/fbisender/src/fileutils"
	"git.asdf.cafe/abs3nt/fbisender/src/server"
)

func SendFiles(ctx context.Context, targetPath string) error {
	config := config.NewConfig()

	fileListPayload, directory, err := prepareFileListPayload(config, targetPath)
	if err != nil {
		return fmt.Errorf("preparing file list payload: %w", err)
	}

	if err = fileutils.ChangeDirectory(directory); err != nil {
		return fmt.Errorf("changing directory: %w", err)
	}

	fmt.Println("\nURLs:")
	fmt.Println(fileListPayload + "\n")

	httpServer := server.StartHTTPServer(config.HostPort)
	defer server.ShutdownHTTPServer(httpServer)

	conn, err := sendPayload(config.TargetIP, config.TargetPort, fileListPayload)
	if err != nil {
		return fmt.Errorf("sending payload: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.WaitForInstallation(ctx, conn); err != nil {
			log.Printf("installation process error: %v", err)
		}
		cancel()
	}()

	select {
	case <-ctx.Done():
	case sig := <-sigChan:
		fmt.Printf("\nReceived signal %v. Initiating shutdown...\n", sig)
		cancel()
	}

	wg.Wait()
	fmt.Println("Server shut down successfully.")
	return nil
}

func prepareFileListPayload(config *config.Config, targetPath string) (string, string, error) {
	fmt.Println("Preparing data...")

	baseURL := fmt.Sprintf("%s:%d/", config.HostIP, config.HostPort)
	fileInfo, err := os.Stat(targetPath)
	if err != nil {
		return "", "", fmt.Errorf("stat target path: %w", err)
	}

	if fileInfo.IsDir() {
		return prepareDirectoryPayload(baseURL, targetPath)
	} else if fileutils.HasAcceptedExtension(fileInfo.Name()) {
		encodedPath := encodeFilePath(baseURL, fileInfo.Name())
		return encodedPath, filepath.Dir(targetPath), nil
	}

	return "", "", fmt.Errorf("unsupported file extension. Supported extensions are: %v", fileutils.GetSupportedExtensions())
}

func prepareDirectoryPayload(baseURL, directory string) (string, string, error) {
	files, err := os.ReadDir(directory)
	if err != nil {
		return "", "", fmt.Errorf("reading directory: %w", err)
	}

	var payloadBuilder strings.Builder
	for _, file := range files {
		if !file.IsDir() && fileutils.HasAcceptedExtension(file.Name()) {
			payloadBuilder.WriteString(encodeFilePath(baseURL, file.Name()) + "\n")
		}
	}

	if payloadBuilder.Len() == 0 {
		return "", "", fmt.Errorf("no files with supported extensions to serve")
	}

	strings.TrimSuffix(payloadBuilder.String(), "\n")

	return payloadBuilder.String(), directory, nil
}

func encodeFilePath(baseURL, fileName string) string {
	return baseURL + url.PathEscape(fileName)
}

func sendPayload(targetIP, targetPort, fileListPayload string) (net.Conn, error) {
	fmt.Printf("Sending URL(s) to %s on port %s...\n", targetIP, targetPort)

	conn, err := net.Dial("tcp", net.JoinHostPort(targetIP, targetPort))
	if err != nil {
		return nil, fmt.Errorf("dialing target device: %w", err)
	}

	payloadBytes := []byte(fileListPayload)
	length := uint32(len(payloadBytes))
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, length)

	if _, err := conn.Write(append(lengthBytes, payloadBytes...)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("writing to connection: %w", err)
	}

	return conn, nil
}
