package internal

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model represents the application state for the selection prompt
type Model struct {
	filter         string
	filteredKeys   []SelectionOption
	allOptions     []SelectionOption
	selected       int
	terminalWidth  int
	terminalHeight int
	scrollOffset   int
	addNewOption   bool
}

var (
	// Style definitions
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7CB9E8")). // Light blue
			Bold(true)

	filterLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF69B4")). // Hot pink
				Bold(true)

	filterTextStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#98FB98")) // Pale green

	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")). // White text
				Background(lipgloss.Color("#4A90E2")). // Softer blue background
				Bold(true).
				Padding(0, 1).
				Border(lipgloss.NormalBorder(), false, false, false, true). // Left border only
				BorderForeground(lipgloss.Color("#FFFFFF"))                 // White border

	regularItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E6E6FA")). // Light lavender
				Padding(0, 1)

	noMatchesStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B")). // Coral red
			Italic(true)

	quitHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")) // Gold
)

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles user input and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle terminal resize messages
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.terminalWidth = wsm.Width
		m.terminalHeight = wsm.Height
	}

	updateFilter := false

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			// Return quit selection option instead of quitting the program
			m.filteredKeys[m.selected] = SelectionOption{"quit", "-1"}
			return m, tea.Quit // Properly exit the program
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				updateFilter = true
			}
		case "down", "tab":
			// Move the selection cursor down
			if m.selected < len(m.filteredKeys)-1 {
				m.selected++
			}

			// Scroll the view if necessary
			if m.selected >= m.scrollOffset+m.visibleItemsCount() {
				m.scrollOffset++
			}
		case "up", "shift+tab":
			// Move the selection cursor up
			if m.selected > 0 {
				m.selected--
			}

			// Scroll the view if necessary
			if m.selected < m.scrollOffset {
				m.scrollOffset--
			}
		case "enter":
			if m.filteredKeys[m.selected].Key == "add_new" {
				CurdOut("Adding a new anime...")
				m.filteredKeys[m.selected] = SelectionOption{"add_new", "0"}
				return m, tea.Quit
			}
			return m, tea.Quit
		default:
			if len(msg.String()) == 1 && msg.String() >= " " && msg.String() <= "~" {
				m.filter += msg.String()
				updateFilter = true
			}
		}
	}

	if updateFilter {
		m.filterOptions()
		m.selected = 0     // Reset selection to the first item after filtering
		m.scrollOffset = 0 // Reset scrolling
	}

	return m, nil
}

// View renders the UI and only shows as many options as fit in the terminal
func (m Model) View() string {
	var b strings.Builder

	// Display the search prompt and filter with colors
	b.WriteString(titleStyle.Render("Search") + " (Press " +
		quitHintStyle.Render("Ctrl+C") + " to quit):\n")

	b.WriteString(filterLabelStyle.Render("Filter: ") +
		filterTextStyle.Render(m.filter) + "\n\n") // Added extra newline for spacing

	if len(m.filteredKeys) == 0 {
		b.WriteString(noMatchesStyle.Render("No matches found.") + "\n")
	} else {
		visibleItems := m.visibleItemsCount()
		start := m.scrollOffset
		end := start + visibleItems
		if end > len(m.filteredKeys) {
			end = len(m.filteredKeys)
		}

		// Render the options within the visible range
		for i := start; i < end; i++ {
			if i == m.selected {
				b.WriteString(selectedItemStyle.Render(m.filteredKeys[i].Label) + "\n")
			} else {
				b.WriteString(regularItemStyle.Render(m.filteredKeys[i].Label) + "\n")
			}
		}
	}

	return b.String()
}

// visibleItemsCount calculates how many options fit in the terminal
func (m Model) visibleItemsCount() int {
	// Leave space for the filter and other UI elements
	return m.terminalHeight - 4 // Adjust this number based on your terminal layout
}

// filterOptions filters and sorts options based on the search term
func (m *Model) filterOptions() {
	m.filteredKeys = nil
	for _, opt := range m.allOptions {
		if strings.Contains(strings.ToLower(opt.Label), strings.ToLower(m.filter)) {
			m.filteredKeys = append(m.filteredKeys, opt)
		}
	}

	// Sort the filtered options alphabetically unless this is a menu selection
	isMenu := false
	for _, opt := range m.allOptions {
		if opt.Key == "ALL" || opt.Key == "CURRENT" {
			isMenu = true
			break
		}
	}

	if !isMenu {
		sort.Slice(m.filteredKeys, func(i, j int) bool {
			return m.filteredKeys[i].Label < m.filteredKeys[j].Label
		})
	}

	// Add "Add new anime" option if enabled
	if m.addNewOption {
		m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: "Add new anime", Key: "add_new"})
	}
	m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: "Quit", Key: "-1"})
}

func DynamicSelectPreview(options map[string]RofiSelectPreview, addnewoption bool) (SelectionOption, error) {
	go preDownloadImages(options, 14)

	userCurdConfig := GetGlobalConfig()
	if userCurdConfig.StoragePath == "" {
		userCurdConfig.StoragePath = os.ExpandEnv("${HOME}/.local/share/curd")
	}

	var rofiInput strings.Builder
	selectionOptions := make([]SelectionOption, 0, len(options))

	// Prepare options and rofi input
	for id, opt := range options {
		cachePath, err := downloadToCache(opt.CoverImage)
		if err != nil {
			Log(fmt.Sprintf("Error caching image: %v", err))
			continue
		}
		rofiInput.WriteString(fmt.Sprintf("%s\x00icon\x1f%s\n", opt.Title, cachePath))
		selectionOptions = append(selectionOptions, SelectionOption{
			Label: opt.Title,
			Key:   id,
		})
	}

	if addnewoption {
		rofiInput.WriteString("Add new anime\n")
	}
	rofiInput.WriteString("Quit\n")

	// Run rofi
	configPath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "selectanimepreview.rasi")
	cmd := exec.Command("rofi", "-dmenu", "-theme", configPath, "-show-icons", "-p", "Select Anime", "-i", "-no-custom")
	cmd.Stdin = strings.NewReader(rofiInput.String())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		Log(fmt.Sprintf("Rofi stderr: %s", stderr.String()))
		Log(fmt.Sprintf("Rofi stdout: %s", stdout.String()))
		return SelectionOption{}, fmt.Errorf("failed to execute rofi: %w", err)
	}

	selected := strings.TrimSpace(stdout.String())

	// Handle special cases
	switch selected {
	case "":
		return SelectionOption{}, fmt.Errorf("no selection made")
	case "Add new anime":
		return SelectionOption{Label: "Add new anime", Key: "add_new"}, nil
	case "Quit":
		return SelectionOption{Label: "Quit", Key: "-1"}, nil
	}

	// Find matching option
	for _, opt := range selectionOptions {
		if opt.Label == selected {
			return opt, nil
		}
	}

	return SelectionOption{}, fmt.Errorf("selection not found in options")
}

func preDownloadImages(options map[string]RofiSelectPreview, count int) {
	i := 0
	for _, option := range options {
		if i >= count {
			break
		}
		downloadToCache(option.CoverImage)
		i++
	}
}

func downloadToCache(imageURL string) (string, error) {
	cacheDir := os.ExpandEnv("${HOME}/.cache/curd/images")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Create a hash of the URL to use as filename
	filename := fmt.Sprintf("%x.jpg", md5.Sum([]byte(imageURL)))
	cachePath := filepath.Join(cacheDir, filename)

	// Check if file already exists in cache
	if _, err := os.Stat(cachePath); err == nil {
		return cachePath, nil
	}

	// Download the image
	resp, err := http.Get(imageURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	file, err := os.Create(cachePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		os.Remove(cachePath) // Clean up on error
		return "", err
	}

	return cachePath, nil
}

func showCachedImagePreview(imageURL string) error {
	cachePath, err := downloadToCache(imageURL)
	if err != nil {
		return err
	}

	// Display the image with ueberzugpp
	cmd := exec.Command("ueberzugpp", "layer", "--silent", "add", "preview", "--path", cachePath)
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start image preview: %w", err)
	}
	time.Sleep(2 * time.Second) // Allow image to load for a moment
	return nil
}

func RofiSelect(options []SelectionOption) (SelectionOption, error) {
	userCurdConfig := GetGlobalConfig()
	if userCurdConfig.StoragePath == "" {
		userCurdConfig.StoragePath = os.ExpandEnv("${HOME}/.local/share/curd")
	}

	// Create ordered list of options
	var optionsList []string
	for _, opt := range options {
		optionsList = append(optionsList, opt.Label)
	}

	// Add "Quit" option at the end
	optionsList = append(optionsList, "Quit")
	optionsString := strings.Join(optionsList, "\n")

	// Prepare and run rofi
	configPath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "selectanime.rasi")
	cmd := exec.Command("rofi", "-dmenu", "-theme", configPath, "-i", "-p", "Select")
	cmd.Stdin = strings.NewReader(optionsString)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			return SelectionOption{Key: "-1", Label: ""}, nil
		}
		return SelectionOption{}, fmt.Errorf("failed to run Rofi: %v", err)
	}

	selected := strings.TrimSpace(stdout.String())

	// Handle special cases
	switch selected {
	case "":
		return SelectionOption{Key: "-1", Label: ""}, nil
	case "Quit":
		return SelectionOption{Label: "Quit", Key: "-1"}, nil
	}

	// Find the matching SelectionOption
	for _, opt := range options {
		if opt.Label == selected {
			return opt, nil
		}
	}

	return SelectionOption{}, fmt.Errorf("selected option not found in original list")
}

// DynamicSelect displays a simple selection prompt without extra features
func DynamicSelect(options []SelectionOption) (SelectionOption, error) {
	if GetGlobalConfig().RofiSelection {
		return RofiSelect(options)
	}

	// Sort options if this contains menu categories
	for _, opt := range options {
		if opt.Key == "ALL" || opt.Key == "CURRENT" {
			menuOrder := strings.Split(GetGlobalConfig().MenuOrder, ",")
			optMap := make(map[string]SelectionOption)
			for _, opt := range options {
				optMap[opt.Key] = opt
			}

			sorted := make([]SelectionOption, 0, len(options))
			for _, key := range menuOrder {
				if opt, exists := optMap[key]; exists {
					sorted = append(sorted, opt)
				}
			}
			options = sorted
			break
		}
	}

	model := &Model{
		allOptions: options,
	}
	model.filterOptions() // Initialize filtered options

	p := tea.NewProgram(model)
	finalModel, err := p.Run()
	if err != nil {
		return SelectionOption{}, err
	}

	fmt.Print("\033[?25h") // Show cursor
	fmt.Print("\033[?7h")  // Enable line wrapping

	finalSelectionModel, ok := finalModel.(*Model)
	if !ok {
		return SelectionOption{}, fmt.Errorf("unexpected model type")
	}

	if finalSelectionModel.selected < len(finalSelectionModel.filteredKeys) {
		return finalSelectionModel.filteredKeys[finalSelectionModel.selected], nil
	}
	return SelectionOption{}, nil
}
