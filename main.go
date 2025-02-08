package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// Command represents a single command
type Command struct {
	Name      string
	Command   string
	Repeat    int // Interval in seconds for repeating jobs (0 = run once)
	Output    string
	Status    string
	IsRunning bool
}

// Group represents a group of commands
type Group struct {
	Repeating    []*Command
	NonRepeating []*Command
}

// AppState holds the app's state
type AppState struct {
	Groups      []*Group
	TextViews   [][]*tview.TextView
	CancelFuncs map[[2]int]context.CancelFunc
	Mu          sync.Mutex
}

// ExecuteCommand runs the command and updates the output
func ExecuteCommand(ctx context.Context, cmd *Command, output *tview.TextView, mu *sync.Mutex, app *tview.Application) {
	for {
		select {
		case <-ctx.Done():
			// Stop execution if the context is canceled
			// mu.Lock()
			// cmd.Status = "Killed"
			// cmd.Output = "Job terminated."
			// content := fmt.Sprintf("Command: %s\nStatus: %s\nOutput:\n%s", cmd.Command, cmd.Status, cmd.Output)
			// mu.Unlock()
			// app.QueueUpdateDraw(func() {
			// 	output.SetText(content)
			// })
			log.Println("cancelling", cmd.Command)
			return
		default:
			var outputBuf bytes.Buffer
			execCmd := exec.Command("sh", "-c", cmd.Command)
			execCmd.Stdout = &outputBuf
			execCmd.Stderr = &outputBuf
			err := execCmd.Run()

			status := "Completed"
			if err != nil {
				status = "Failed"
			}

			// Update the command's output and status
			mu.Lock()
			cmd.Output = outputBuf.String()
			cmd.Status = status
			content := fmt.Sprintf("Command: %s\nStatus: %s\nOutput:\n%s", cmd.Command, cmd.Status, cmd.Output)
			mu.Unlock()

			// Refresh the TextView on the UI thread
			app.QueueUpdateDraw(func() {
				output.SetText(content)
			})

			// Sleep if the job is repeating
			if cmd.Repeat > 0 {
				time.Sleep(time.Duration(cmd.Repeat) * time.Second)
			} else {
				return
			}
		}
	}
}

// GroupCommands groups commands into logical groups
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

		groups = append(groups, group)
	}

	return groups
}

// CreateGroupedFlex initializes a grouped layout for the app
func CreateGroupedFlex(state *AppState) []*tview.Flex {
	groups := []*tview.Flex{}

	for groupIndex, group := range state.Groups {
		groupFlex := tview.NewFlex().SetDirection(tview.FlexRow)

		// Initialize the sub-slice for this group
		state.TextViews[groupIndex] = make([]*tview.TextView, 0)

		// Add repeating commands (vertically stacked)
		for _, cmd := range group.Repeating {
			textView := tview.NewTextView().
				SetDynamicColors(true)

			textView.SetBorder(true)
			textView.SetTitle(fmt.Sprintf("Repeating: %s", cmd.Name))
			textView.SetBorderColor(tcell.ColorWhite)

			state.TextViews[groupIndex] = append(state.TextViews[groupIndex], textView)
			groupFlex.AddItem(textView, 0, 1, false)
		}

		// Add non-repeating commands (horizontally stacked)
		nonRepeatingFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
		for _, cmd := range group.NonRepeating {
			textView := tview.NewTextView().
				SetDynamicColors(true)

			textView.SetBorder(true)
			textView.SetTitle(fmt.Sprintf("Non-Repeating: %s", cmd.Name))
			textView.SetBorderColor(tcell.ColorWhite)

			state.TextViews[groupIndex] = append(state.TextViews[groupIndex], textView)
			nonRepeatingFlex.AddItem(textView, 0, 1, false)
		}

		groupFlex.AddItem(nonRepeatingFlex, 0, 1, false)
		groups = append(groups, groupFlex)
	}

	return groups
}

// CreateApp initializes the TUI application
func CreateApp(state *AppState, groups []*tview.Flex, cancel context.CancelFunc) *tview.Application {
	app := tview.NewApplication()

	// Create a Flex layout for vertical stacking
	rootFlex := tview.NewFlex().SetDirection(tview.FlexRow)
	for _, groupItem := range groups {
		rootFlex.AddItem(groupItem, 0, 1, false)
	}

	// Handle key events
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			cancel()
			app.Stop() // Quit the application
		}
		return event
	})

	app.SetRoot(rootFlex, true)
	return app
}

func main() {
	// Define commands
	commands := []*Command{
		{Name: "df", Command: "df -kh", Repeat: 5},
		{Name: "date", Command: "date", Repeat: 1},     // Dynamically updating time
		{Name: "whoami", Command: "whoami", Repeat: 0}, // Run once
		{Name: "whoami", Command: "whoami", Repeat: 0}, // Run once
	}

	// Group commands
	groups := GroupCommands(commands)

	// Initialize app state
	state := &AppState{
		Groups:      groups,
		TextViews:   make([][]*tview.TextView, len(groups)), // Create slices for groups
		CancelFuncs: make(map[[2]int]context.CancelFunc),
	}

	// Initialize grouped layout
	groupItems := CreateGroupedFlex(state)

	// Create the application

	// Start executing commands
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	app := CreateApp(state, groupItems, cancel)

	for groupIndex, group := range groups {
		for paneIndex, cmd := range append(group.Repeating, group.NonRepeating...) {
			wg.Add(1)

			childCtx, childCancel := context.WithCancel(ctx)
			state.CancelFuncs[[2]int{groupIndex, paneIndex}] = childCancel

			go func(cx context.Context, cmd *Command, groupIndex, paneIndex int) {
				defer wg.Done()
				ExecuteCommand(cx, cmd, state.TextViews[groupIndex][paneIndex], &state.Mu, app)
			}(childCtx, cmd, groupIndex, paneIndex)
		}
	}

	// Run the TUI
	go func() {
		if err := app.Run(); err != nil {
			panic(err)
		}
		cancel() // Cancel all running commands when the app exits
	}()

	fmt.Println("Waiting for clean exit")
	// Wait for all tasks to complete
	wg.Wait()
	fmt.Println("All tasks completed. Exiting.")
}
