package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/masterking32/MasterDnsVPN/config"
	"github.com/masterking32/MasterDnsVPN/server"
	"github.com/masterking32/MasterDnsVPN/client"
)

const (
	Version = "1.0.0"
	AppName = "MasterDnsVPN"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "config.toml", "Path to configuration file")
	// Changed default mode to "server" since I primarily run this as a server
	mode := flag.String("mode", "server", "Run mode: client or server")
	showVersion := flag.Bool("version", false, "Show version information")
	// Default verbose to false to reduce noise in normal operation
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	flag.Parse()

	if *showVersion {
		fmt.Printf("%s v%s\n", AppName, Version)
		os.Exit(0)
	}

	log.Printf("Starting %s v%s in %s mode", AppName, Version, *mode)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if *verbose {
		log.Printf("Configuration loaded from: %s", *configPath)
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the appropriate mode
	switch *mode {
	case "server":
		srv, err := server.New(cfg)
		if err != nil {
			log.Fatalf("Failed to initialize server: %v", err)
		}
		go func() {
			if err := srv.Start(); err != nil {
				log.Fatalf("Server error: %v", err)
			}
		}()
		log.Printf("Server started successfully")

	case "client":
		cli, err := client.New(cfg)
		if err != nil {
			log.Fatalf("Failed to initialize client: %v", err)
		}
		go func() {
			if err := cli.Connect(); err != nil {
				log.Fatalf("Client connection error: %v", err)
			}
		}()
		log.Printf("Client connected successfully")

	default:
		log.Fatalf("Unknown mode: %s. Use 'client' or 'server'", *mode)
	}

	// Wait for shutdown signal
	sig := <-sigChan
	log.Printf("Received signal %v, shutting down...", sig)
}
