package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"gopkg.in/yaml.v3"
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

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	isDebug := os.Getenv("DEBUG") == "1"
	if !isDebug {
		return
	}

	logFile, err := os.OpenFile("app.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		os.Exit(1)
	}

	log.SetOutput(logFile)
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

			// log.Println("out", err, outputBuf.String())
			// Update the command's output and status
			mu.Lock()
			cmd.Status = status

			if err != nil {
				cmd.Status = err.Error()
			} else {
				cmd.Output = outputBuf.String()
			}
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
// func CreateGroupedFlex(state *AppState) []*tview.Flex {
// 	groups := []*tview.Flex{}
//
// 	for groupIndex, group := range state.Groups {
// 		log.Printf("Creating group %d with %d repeating and %d non-repeating commands\n",
// 			groupIndex, len(group.Repeating), len(group.NonRepeating))
//
// 		groupFlex := tview.NewFlex().SetDirection(tview.FlexRow)
//
// 		// Initialize the sub-slice for this group
// 		state.TextViews[groupIndex] = make([]*tview.TextView, 0)
//
// 		// Add repeating commands (vertically stacked)
// 		for _, cmd := range group.Repeating {
// 			textView := tview.NewTextView().
// 				SetDynamicColors(true)
//
// 			textView.SetBorder(true)
// 			textView.SetTitle(fmt.Sprintf("Repeating: %s", cmd.Name))
// 			textView.SetBorderColor(tcell.ColorWhite)
//
// 			state.TextViews[groupIndex] = append(state.TextViews[groupIndex], textView)
//
// 			groupFlex.AddItem(textView, 0, 1, false)
// 		}
//
// 		// Add non-repeating commands (horizontally stacked)
// 		nonRepeatingFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
// 		for _, cmd := range group.NonRepeating {
// 			textView := tview.NewTextView().
// 				SetDynamicColors(true)
//
// 			textView.SetBorder(true)
// 			textView.SetTitle(fmt.Sprintf("Non-Repeating: %s", cmd.Name))
// 			textView.SetBorderColor(tcell.ColorWhite)
//
// 			state.TextViews[groupIndex] = append(state.TextViews[groupIndex], textView)
// 			nonRepeatingFlex.AddItem(textView, 0, 1, false)
// 		}
//
// 		groupFlex.AddItem(nonRepeatingFlex, 0, 1, false)
// 		groups = append(groups, groupFlex)
// 	}
//
// 	return groups
// }

func CreateGroupedFlex(state *AppState) []*tview.Flex {
	groups := []*tview.Flex{}

	for groupIndex, group := range state.Groups {
		log.Printf("Creating group %d with %d repeating and %d non-repeating commands\n",
			groupIndex, len(group.Repeating), len(group.NonRepeating))

		// Group-level flex container (vertical stacking)
		groupFlex := tview.NewFlex().SetDirection(tview.FlexRow)

		// Initialize the sub-slice for this group
		state.TextViews[groupIndex] = make([]*tview.TextView, 0)

		// Add repeating commands (each in its own row)
		for _, cmd := range group.Repeating {
			textView := tview.NewTextView().
				SetDynamicColors(true)

			textView.SetBorder(true)
			textView.SetTitle(fmt.Sprintf("Repeating: %s", cmd.Name))
			textView.SetBorderColor(tcell.ColorGreen)

			state.TextViews[groupIndex] = append(state.TextViews[groupIndex], textView)
			groupFlex.AddItem(textView, 0, 1, false) // Each repeating command gets a row
		}

		// Add non-repeating commands (2 per row)
		if len(group.NonRepeating) > 0 {
			nonRepeatingFlex := tview.NewFlex().SetDirection(tview.FlexColumn)
			for i, cmd := range group.NonRepeating {
				textView := tview.NewTextView().
					SetDynamicColors(true)

				textView.SetBorder(true)
				textView.SetTitle(fmt.Sprintf("Non-Repeating: %s", cmd.Name))
				textView.SetBorderColor(tcell.ColorBlue)

				state.TextViews[groupIndex] = append(state.TextViews[groupIndex], textView)
				nonRepeatingFlex.AddItem(textView, 0, 1, false)

				// Every 2 commands, finalize the row and start a new one
				if (i+1)%2 == 0 || i == len(group.NonRepeating)-1 {
					groupFlex.AddItem(nonRepeatingFlex, 0, 1, false)
					nonRepeatingFlex = tview.NewFlex().SetDirection(tview.FlexColumn)
				}
			}
		}

		// Add the group to the main layout
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

type YAMLCommand struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
	Repeat  int    `yaml:"repeat"`
}

type YAMLConfig struct {
	Commands []YAMLCommand `yaml:"commands"`
}

// LoadCommandsFromYAML parses the YAML file and returns a list of commands
func LoadCommandsFromYAML(filename string) ([]*Command, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open YAML file: %v", err)
	}
	defer file.Close()

	var config YAMLConfig
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode YAML file: %v", err)
	}

	var commands []*Command
	for _, yamlCmd := range config.Commands {
		commands = append(commands, &Command{
			Name:    yamlCmd.Name,
			Command: yamlCmd.Command,
			Repeat:  yamlCmd.Repeat,
		})
	}

	return commands, nil
}

type paginator struct {
	total   int32
	current int32
	mu      *sync.Mutex
}

func newPaginator(total int32) *paginator {
	return &paginator{
		total:   total,
		current: 0,
		mu:      &sync.Mutex{},
	}
}

func (p *paginator) next() int32 {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = (p.current + 1) % p.total
	return p.current
}

func (p *paginator) prev() int32 {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.current = (p.current - 1 + p.total) % p.total
	return p.current
}

func main() {
	var filePaths string

	// Accept comma-separated YAML file paths
	flag.StringVar(&filePaths, "cfg", "", "provide comma-separated commands config yaml files")
	flag.Parse()

	if filePaths == "" {
		log.Fatal("no commands files provided")
	}

	// Parse files
	files := strings.Split(filePaths, ",")
	if len(files) == 0 {
		log.Fatal("no valid files found")
	}

	// Initialize TUI components
	app := tview.NewApplication()
	pages := tview.NewPages()

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	cursor := newPaginator(int32(len(files)))

	// Process each file
	for fileIndex, filePath := range files {
		// Load commands from YAML
		commands, err := LoadCommandsFromYAML(filePath)
		if err != nil {
			log.Fatalf("failed to decode yaml from file %s. error: %v", filePath, err)
		}

		// Group commands
		groups := GroupCommands(commands)

		// Initialize state for this file
		state := &AppState{
			Groups:      groups,
			TextViews:   make([][]*tview.TextView, len(groups)),
			CancelFuncs: make(map[[2]int]context.CancelFunc),
		}

		// Create grouped layout for this file
		groupItems := CreateGroupedFlex(state)

		pageTitle := tview.NewTextView().
			SetDynamicColors(true).
			SetTextAlign(tview.AlignCenter).
			SetText(fmt.Sprintf("[::b]Page %d: %s[-:-:-]", fileIndex+1, filePath))

		pageTitle.SetBorder(true)
		pageTitle.SetBorderColor(tcell.ColorYellow)

		// Add a new page for this file
		page := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(pageTitle, 3, 1, false)

		for _, group := range groupItems {
			page.AddItem(group, 0, 1, false)
		}
		pages.AddPage(fmt.Sprintf("file-%d", fileIndex), page, true, fileIndex == 0)

		// Execute commands for this file
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
	}

	// Set up navigation between pages
	pages.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {
		case 'q':
			cancel()
			app.Stop()
		case 'n': // Next page
			page := cursor.next()
			log.Println("next page", page)
			pages.SwitchToPage(fmt.Sprintf("file-%d", page))
		case 'p': // Previous page
			page := cursor.prev()
			log.Println("prev page", page)
			pages.SwitchToPage(fmt.Sprintf("file-%d", page))
		}
		return event
	})

	// Run the TUI
	go func() {
		if err := app.SetRoot(pages, true).Run(); err != nil {
			panic(err)
		}
		cancel()
	}()

	fmt.Println("Waiting for clean exit")
	wg.Wait()
	fmt.Println("All tasks completed. Exiting.")
}
