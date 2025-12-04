// Package main is the entry point for the Alexander Storage admin CLI.
// This tool provides administrative commands for managing users, access keys, and system operations.
package main

import (
	"fmt"
	"os"
)

// Version information (set at build time)
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "version":
		fmt.Printf("Alexander Storage Admin CLI\n")
		fmt.Printf("Version: %s\n", Version)
		fmt.Printf("Build Time: %s\n", BuildTime)
		fmt.Printf("Git Commit: %s\n", GitCommit)

	case "user":
		// TODO: User management commands
		fmt.Println("User management - not yet implemented")

	case "accesskey":
		// TODO: Access key management commands
		fmt.Println("Access key management - not yet implemented")

	case "bucket":
		// TODO: Bucket management commands
		fmt.Println("Bucket management - not yet implemented")

	case "gc":
		// TODO: Garbage collection commands
		fmt.Println("Garbage collection - not yet implemented")

	case "help", "-h", "--help":
		printUsage()

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Alexander Storage Admin CLI

Usage:
  alexander-admin <command> [arguments]

Commands:
  user        Manage users (create, list, delete, update)
  accesskey   Manage access keys (create, list, revoke, rotate)
  bucket      Manage buckets (list, delete, set-versioning)
  gc          Run garbage collection for orphan blobs
  version     Print version information
  help        Show this help message

Examples:
  alexander-admin user create --username admin --email admin@example.com
  alexander-admin accesskey create --user-id <uuid>
  alexander-admin bucket list --owner <user-id>
  alexander-admin gc run --dry-run

Use "alexander-admin <command> --help" for more information about a command.`)
}
