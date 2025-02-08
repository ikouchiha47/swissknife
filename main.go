package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Command struct to hold the command details
type Command struct {
	Name      string
	Command   string
	Repeat    int // Repeat interval in seconds (0 means run once)
	Output    string
	Status    string
	IsRunning bool
}

// Group struct to group commands
type Group struct {
	Repeating    []*Command
	NonRepeating []*Command
}

// ExecuteCommand runs the actual shell command and updates the Command struct
func executeCommand(cmd *Command, mu *sync.Mutex) {
	var outputBuf bytes.Buffer

	// Run the command
	command := exec.Command("sh", "-c", cmd.Command)
	command.Stdout = &outputBuf
	command.Stderr = &outputBuf

	err := command.Run()
	status := "Completed"
	if err != nil {
		status = "Failed"
	}

	// Update the Command struct
	mu.Lock()
	cmd.Output = outputBuf.String()
	cmd.Status = status
	mu.Unlock()
}

// RunCommand executes commands based on their repeat settings
func RunCommand(ctx context.Context, cmd *Command, wg *sync.WaitGroup, mu *sync.Mutex) {
	defer wg.Done()

	if cmd.Repeat > 0 {
		// Repeating command
		ticker := time.NewTicker(time.Duration(cmd.Repeat) * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				// Exit on context cancellation
				return
			case <-ticker.C:
				executeCommand(cmd, mu)
			}
		}
	} else {
		// One-time command
		executeCommand(cmd, mu)
	}
}

// RefreshDisplay periodically clears the terminal and prints the grouped output
func RefreshDisplay(ctx context.Context, groups []*Group, mu *sync.Mutex) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Exit on context cancellation
			return
		case <-ticker.C:
			// Clear the screen (ANSI escape sequence)
			fmt.Print("\033[H\033[2J")

			// Print each group
			for i, group := range groups {
				fmt.Printf("Group %d:\n", i+1)

				// Repeating commands (vertically stacked)
				fmt.Println("Repeating Commands:")
				for _, cmd := range group.Repeating {
					mu.Lock()
					fmt.Printf("Command: %s\nStatus: %s\n\nOutput:\n%s\n\n%s\n",
						cmd.Command, cmd.Status, cmd.Output, "----------------------------------------")
					mu.Unlock()
				}

				// Non-repeating commands (horizontally split)
				fmt.Println("Non-Repeating Commands:")
				mu.Lock()
				for _, cmd := range group.NonRepeating {
					fmt.Printf("\nCommand: %s\nStatus: %s\nOutput:\n%s\n", cmd.Command, cmd.Status, cmd.Output)
					fmt.Println("************************************")
				}
				mu.Unlock()

				fmt.Println("\n========================================\n")
			}
		}
	}
}

// GroupCommands groups commands into logical groups based on the requirements
func GroupCommands(commands []*Command) []*Group {
	var groups []*Group
	var repeating []*Command
	var nonRepeating []*Command

	// Separate repeating and non-repeating commands
	for _, cmd := range commands {
		if cmd.Repeat > 0 {
			repeating = append(repeating, cmd)
		} else {
			nonRepeating = append(nonRepeating, cmd)
		}
	}

	// Create groups of 1 repeating + 2 non-repeating
	for len(repeating) > 0 || len(nonRepeating) > 0 {
		group := &Group{}

		// Add 1 repeating command to the group
		if len(repeating) > 0 {
			group.Repeating = append(group.Repeating, repeating[0])
			repeating = repeating[1:]
		}

		// Add up to 2 non-repeating commands to the group
		for i := 0; i < 2 && len(nonRepeating) > 0; i++ {
			group.NonRepeating = append(group.NonRepeating, nonRepeating[0])
			nonRepeating = nonRepeating[1:]
		}

		// Add the group to the list
		groups = append(groups, group)
	}

	return groups
}

func main() {
	// Define the commands
	commands := []*Command{
		{
			Name:    "df",
			Command: "df -kh",
			Repeat:  5, // Repeat every 5 seconds
		},
		{
			Name:    "lsof",
			Command: "lsof | grep ESTABLISHED",
			Repeat:  0, // Run once
		},
		{
			Name:    "uptime",
			Command: "uptime",
			Repeat:  0, // Run once
		},
		{
			Name:    "free",
			Command: "free -h",
			Repeat:  5, // Repeat every 5 seconds
		},
		{
			Name:    "whoami",
			Command: "whoami",
			Repeat:  0, // Run once
		},
	}

	// Group the commands
	groups := GroupCommands(commands)

	// Mutex to handle concurrent writes to the Command struct
	var mu sync.Mutex

	// WaitGroup to manage Goroutines
	var wg sync.WaitGroup

	// Context to handle cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Signal handling for graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the commands
	for _, group := range groups {
		for _, cmd := range group.Repeating {
			wg.Add(1)
			go RunCommand(ctx, cmd, &wg, &mu)
		}
		for _, cmd := range group.NonRepeating {
			wg.Add(1)
			go RunCommand(ctx, cmd, &wg, &mu)
		}
	}

	// Start the display refresh
	go RefreshDisplay(ctx, groups, &mu)

	// Wait for termination signal
	<-signalChan
	fmt.Println("\nShutting down...")

	// Cancel the context to stop all Goroutines
	cancel()

	// Wait for all Goroutines to finish
	wg.Wait()

	fmt.Println("All tasks completed. Exiting.")
}
