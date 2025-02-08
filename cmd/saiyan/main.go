package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"plugin"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Plugin interface for extensibility
type Plugin interface {
	Name() string
	LoadCommands() ([]Command, error)
}

// Command represents a single command
type Command struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
	Repeat  int    `yaml:"repeat"` // Repeat interval in seconds (0 means run once)
}

// Job represents a background job with its state
type Job struct {
	Name      string
	Command   string
	Output    string
	Status    string
	LastRun   time.Time
	IsRunning bool
	Repeat    int
	NextRun   time.Time
}

// Model for Bubble Tea
type model struct {
	jobs   []Job
	width  int
	height int
}

const (
	zenburnBackground = "#3F3F3F"
	zenburnForeground = "#DCDCCC"
	zenburnBorder     = "#6F6F6F"
	zenburnHighlight  = "#F0DFAF"
	zenburnSuccess    = "#7F9F7F"
	zenburnError      = "#CC9393"
	zenburnWarning    = "#DFAF8F"
)

// Styles for rendering tiles
// var (
// 	baseStyle     = lipgloss.NewStyle().Padding(1).Background(lipgloss.Color(zenburnBackground)).Foreground(lipgloss.Color(zenburnForeground))
// 	tileStyle     = baseStyle.Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color(zenburnBorder))
// 	selectedTile  = tileStyle.BorderForeground(lipgloss.Color(zenburnHighlight))
// 	runningTile   = tileStyle.BorderForeground(lipgloss.Color(zenburnWarning))
// 	completedTile = tileStyle.BorderForeground(lipgloss.Color(zenburnSuccess))
// 	failedTile    = tileStyle.BorderForeground(lipgloss.Color(zenburnError))
// 	cronJobTile   = tileStyle.BorderForeground(lipgloss.Color(zenburnHighlight)).Bold(true)
// )

var (
	tileStyle     = lipgloss.NewStyle().Padding(1).Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("63"))
	selectedTile  = tileStyle.BorderForeground(lipgloss.Color("205"))
	defaultTile   = tileStyle.BorderForeground(lipgloss.Color("240"))
	runningTile   = tileStyle.BorderForeground(lipgloss.Color("33"))
	completedTile = tileStyle.BorderForeground(lipgloss.Color("34"))
	failedTile    = tileStyle.BorderForeground(lipgloss.Color("160"))
	cronJobTile   = tileStyle.BorderForeground(lipgloss.Color("45"))
)

// Message types for updating job state
type jobUpdateMsg struct {
	Index   int
	Output  string
	Status  string
	IsCron  bool
	NextRun time.Time
}

// Load commands from YAML
func loadCommands(filename string) ([]Command, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config struct {
		Commands []Command `yaml:"commands"`
	}
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return config.Commands, nil
}

// Init initializes the TUI
func (m model) Init() tea.Cmd {
	// Start all commands (including cron jobs)
	var cmds []tea.Cmd
	for i := range m.jobs {
		cmds = append(cmds, runJob(i, m.jobs[i]))
	}
	return tea.Batch(cmds...)
}

// Update handles messages and state updates
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			return m, tea.Quit // Quit the application
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case jobUpdateMsg:
		// Update the state of a specific job
		m.jobs[msg.Index].Output = msg.Output
		m.jobs[msg.Index].Status = msg.Status
		m.jobs[msg.Index].IsRunning = false
		m.jobs[msg.Index].LastRun = time.Now()
		m.jobs[msg.Index].NextRun = msg.NextRun

		// Schedule the next run for repeating jobs
		if msg.IsCron && msg.NextRun.After(time.Now()) {
			return m, scheduleJob(msg.Index, msg.NextRun)
		}
	}
	return m, nil
}

// View renders the TUI
func (m model) View() string {
	repeatingJobs := []Job{}
	nonRepeatingJobs := []Job{}

	// Split jobs into repeating and non-repeating
	for _, job := range m.jobs {
		if job.Repeat > 0 {
			repeatingJobs = append(repeatingJobs, job)
		} else {
			nonRepeatingJobs = append(nonRepeatingJobs, job)
		}
	}

	// Render layout based on the number of jobs
	if len(m.jobs) == 1 {
		return renderSingleTile(m.jobs[0], m.width, m.height)
	} else if len(m.jobs) == 2 {
		return renderTwoTiles(m.jobs, m.width, m.height)
	} else {
		return renderComplexLayout(nonRepeatingJobs, repeatingJobs, m.width, m.height)
	}
}

// Render a single tile that takes the full screen
func renderSingleTile(job Job, width, height int) string {
	tile := formatJobTile(job, width, height)
	return lipgloss.NewStyle().Width(width).Height(height).Render(tile)
}

// Render two tiles split vertically
func renderTwoTiles(jobs []Job, width, height int) string {
	topTile := formatJobTile(jobs[0], width, height/2)
	bottomTile := formatJobTile(jobs[1], width, height/2)
	return lipgloss.JoinVertical(lipgloss.Top, topTile, bottomTile)
}

// Render a complex layout for 3+ jobs
func renderComplexLayout(nonRepeating, repeating []Job, width, height int) string {
	if len(repeating) > 0 && len(nonRepeating) >= 2 {
		// Repeating job in the lower half, two non-repeating jobs in the upper half
		topLeftTile := formatJobTile(nonRepeating[0], width/2, height/2)
		topRightTile := formatJobTile(nonRepeating[1], width/2, height/2)
		bottomTile := formatJobTile(repeating[0], width, height/2)

		topRow := lipgloss.JoinHorizontal(lipgloss.Top, topLeftTile, topRightTile)
		return lipgloss.JoinVertical(lipgloss.Top, topRow, bottomTile)
	}

	// General case: 3 columns per row
	var rows []string
	colWidth := width / 3
	rowHeight := height / ((len(nonRepeating) + 2) / 3)

	for i, job := range nonRepeating {
		tile := formatJobTile(job, colWidth, rowHeight)
		if i%3 == 0 && i > 0 {
			rows = append(rows, "\n")
		}
		rows = append(rows, tile)
	}

	return lipgloss.JoinVertical(lipgloss.Top, rows...)
}

// Format a single job into a tile
func formatJobTile(job Job, width, height int) string {
	style := tileStyle
	switch job.Status {
	case "Running":
		style = runningTile
	case "Completed":
		style = completedTile
	case "Failed":
		style = failedTile
	}

	if job.Repeat > 0 {
		style = cronJobTile
	}

	return style.Width(width).Height(height).Render(
		fmt.Sprintf("%s\n\nStatus: %s\n\nOutput:\n%s", job.Name, job.Status, job.Output),
	)
}

// runJob executes a job (for both one-off and cron jobs)
func runJob(index int, job Job) tea.Cmd {
	return func() tea.Msg {
		var outputBuf bytes.Buffer
		cmd := exec.Command("sh", "-c", job.Command)
		cmd.Stdout = &outputBuf
		cmd.Stderr = &outputBuf

		// Mark job as running
		job.IsRunning = true
		job.Status = "Running"

		err := cmd.Run()
		status := "Completed"
		if err != nil {
			status = "Failed"
		}

		// Determine next run time for cron jobs
		nextRun := time.Now()
		if job.Repeat > 0 {
			nextRun = nextRun.Add(time.Duration(job.Repeat) * time.Second)
		}

		fmt.Println("response ", job.Name, outputBuf.String())
		return jobUpdateMsg{
			Index:   index,
			Output:  outputBuf.String(),
			Status:  status,
			IsCron:  job.Repeat > 0,
			NextRun: nextRun,
		}
	}
}

// scheduleJob schedules a cron job for its next run
func scheduleJob(index int, nextRun time.Time) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(time.Until(nextRun))
		return runJob(index, Job{
			Name:    "", // No need to redefine for next run
			Command: "",
		})()
	}
}

// Load commands from a YAML file
func loadCommandsFromYAML(filename string) ([]Command, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var commands struct {
		Commands []Command `yaml:"commands"`
	}
	err = yaml.Unmarshal(data, &commands)
	if err != nil {
		return nil, err
	}

	return commands.Commands, nil
}

// Load plugins dynamically
func loadPlugins(pluginDir string) ([]Command, error) {
	commands := []Command{}
	files, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".so") {
			plug, err := plugin.Open(fmt.Sprintf("%s/%s", pluginDir, file.Name()))
			if err != nil {
				log.Printf("Failed to load plugin %s: %v", file.Name(), err)
				continue
			}

			// Lookup Plugin symbols
			symPlugin, err := plug.Lookup("Plugin")
			if err != nil {
				log.Printf("Failed to find Plugin symbol in %s: %v", file.Name(), err)
				continue
			}

			// Assert Plugin interface
			pluginInstance, ok := symPlugin.(Plugin)
			if !ok {
				log.Printf("Invalid plugin type in %s", file.Name())
				continue
			}

			// Load commands from plugin
			pluginCommands, err := pluginInstance.LoadCommands()
			if err != nil {
				log.Printf("Failed to load commands from %s: %v", pluginInstance.Name(), err)
				continue
			}

			commands = append(commands, pluginCommands...)
		}
	}

	return commands, nil
}

// Main function

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: ./tool <commands.yaml>")
		os.Exit(1)
	}

	// Load commands from YAML
	commands, err := loadCommands(os.Args[1])
	if err != nil {
		fmt.Printf("Failed to load commands: %v\n", err)
		os.Exit(1)
	}

	// Convert commands to jobs
	var jobs []Job
	for _, cmd := range commands {
		jobs = append(jobs, Job{
			Name:    cmd.Name,
			Command: cmd.Command,
			Repeat:  cmd.Repeat,
			Status:  "Pending",
		})
	}

	// Initialize the TUI
	initialModel := model{
		jobs:   jobs,
		width:  80,
		height: 24,
	}
	p := tea.NewProgram(initialModel, tea.WithAltScreen())
	if err := p.Start(); err != nil {
		fmt.Printf("Error starting program: %v\n", err)
	}
}

// func main() {
// 	if len(os.Args) < 2 {
// 		fmt.Println("Usage: ./tool <commands.yaml>")
// 		os.Exit(1)
// 	}
//
// 	// Load commands from YAML
// 	commands, err := loadCommandsFromYAML(os.Args[1])
// 	if err != nil {
// 		fmt.Printf("Failed to load commands: %v\n", err)
// 		os.Exit(1)
// 	}
//
// 	// Load commands from plugins
// 	pluginCommands, err := loadPlugins("./plugins")
// 	if err != nil {
// 		fmt.Printf("Failed to load plugins: %v\n", err)
// 	} else {
// 		commands = append(commands, pluginCommands...)
// 	}
//
// 	// Initialize Bubble Tea
// 	initialModel := model{
// 		commands: commands,
// 		width:    80,
// 		height:   24,
// 	}
// 	p := tea.NewProgram(initialModel, tea.WithAltScreen())
// 	if err := p.Start(); err != nil {
// 		fmt.Printf("Error starting program: %v\n", err)
// 	}
// }
