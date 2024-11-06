package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	defaultHostPort   = 8080
	defaultTargetPort = "5000"
)

var acceptedExtensions = []string{".cia", ".tik", ".cetk", ".3dsx"}

type Config struct {
	TargetIP string `yaml:"target_ip"`
	HostIP   string `yaml:"host_ip"`
	HostPort int    `yaml:"host_port"`
}

func main() {
	log.SetFlags(0)

	config, err := loadConfig()
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	targetPath, err := parseArguments()
	if err != nil {
		log.Fatalf("Error parsing arguments: %v", err)
	}

	fileListPayload, directory, err := prepareFileListPayload(config, targetPath)
	if err != nil {
		log.Fatalf("Error preparing file list payload: %v", err)
	}

	if err := changeDirectory(directory); err != nil {
		log.Fatalf("Error changing directory: %v", err)
	}

	fmt.Println("\nURLs:")
	fmt.Println(fileListPayload + "\n")

	server := startHTTPServer(config.HostPort)

	conn, err := sendPayload(config.TargetIP, fileListPayload)
	if err != nil {
		log.Fatalf("Error sending payload: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := waitForInstallation(ctx, conn); err != nil {
			log.Printf("Installation process error: %v", err)
		}
		cancel()
	}()

	select {
	case <-ctx.Done():
	case sig := <-sigChan:
		fmt.Printf("\nReceived signal %v. Shutting down...\n", sig)
		cancel()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	wg.Wait()

	fmt.Println("Server gracefully shut down.")
}

func loadConfig() (*Config, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home directory: %w", err)
		}
		configDirectory = filepath.Join(homeDir, ".config")
	}
	data, err := os.ReadFile(filepath.Join(configDirectory, "fbisender", "config.yaml"))
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	if config.TargetIP == "" {
		return nil, err
	}

	if config.HostIP == "" {
		log.Println("Detecting host IP...")
		hostIP, err := detectHostIP()
		if err != nil {
			return nil, err
		}
		config.HostIP = hostIP
	}

	if config.HostPort == 0 {
		config.HostPort = defaultHostPort
	}

	return &config, nil
}

func parseArguments() (string, error) {
	if len(os.Args) < 2 {
		return "", fmt.Errorf("usage: %s <file / directory>", os.Args[0])
	}

	targetPath := strings.TrimSpace(os.Args[1])
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return "", fmt.Errorf("%s: no such file or directory", targetPath)
	}

	return targetPath, nil
}

func prepareFileListPayload(config *Config, targetPath string) (string, string, error) {
	fmt.Println("Preparing data...")

	baseURL := config.HostIP + ":" + strconv.Itoa(config.HostPort) + "/"
	var fileListPayload string
	var directory string

	fileInfo, err := os.Stat(targetPath)
	if err != nil {
		return "", "", err
	}

	if fileInfo.IsDir() {
		directory = targetPath
		files, err := os.ReadDir(directory)
		if err != nil {
			return "", "", fmt.Errorf("reading directory: %w", err)
		}

		for _, file := range files {
			if !file.IsDir() && hasAcceptedExtension(file.Name()) {
				encodedFileName := url.PathEscape(file.Name())
				fileListPayload += baseURL + encodedFileName + "\n"
			}
		}
	} else {
		if hasAcceptedExtension(fileInfo.Name()) {
			encodedFileName := url.PathEscape(fileInfo.Name())
			fileListPayload = baseURL + encodedFileName
			directory = filepath.Dir(targetPath)
		} else {
			return "", "", fmt.Errorf("unsupported file extension. Supported extensions are: %v", acceptedExtensions)
		}
	}

	if fileListPayload == "" {
		return "", "", fmt.Errorf("no files to serve")
	}

	return fileListPayload, directory, nil
}

func changeDirectory(directory string) error {
	if directory != "" && directory != "." {
		if err := os.Chdir(directory); err != nil {
			return err
		}
	}
	return nil
}

func startHTTPServer(port int) *http.Server {
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

func sendPayload(targetIP, fileListPayload string) (net.Conn, error) {
	fmt.Printf("Sending URL(s) to %s on port %s...\n", targetIP, defaultTargetPort)

	conn, err := net.Dial("tcp", net.JoinHostPort(targetIP, defaultTargetPort))
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

func waitForInstallation(ctx context.Context, conn net.Conn) error {
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

func detectHostIP() (string, error) {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return "", fmt.Errorf("detecting host IP: %w", err)
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

func hasAcceptedExtension(fileName string) bool {
	ext := strings.ToLower(filepath.Ext(fileName))
	for _, acceptedExt := range acceptedExtensions {
		if ext == acceptedExt {
			return true
		}
	}
	return false
}
