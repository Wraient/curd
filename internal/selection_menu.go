package internal

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
	"github.com/charmbracelet/bubbletea"
	"crypto/md5"
	"path/filepath"
)

// SelectionOption holds the label and the internal key
type SelectionOption struct {
	Label string
	Key   string
}

// Model represents the application state for the selection prompt
type Model struct {
	options        map[string]string
	filter         string
	filteredKeys   []SelectionOption
	selected       int
	terminalWidth  int
	terminalHeight int
	scrollOffset   int // Track the topmost visible item
}

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
		case "ctrl+c", "q":
			// Return quit selection option instead of quitting the program
			m.filteredKeys[m.selected] = SelectionOption{"quit", "-1"}
			return m, tea.Quit // Properly exit the program
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				updateFilter = true
			}
		case "down":
			// Move the selection cursor down
			if m.selected < len(m.filteredKeys)-1 {
				m.selected++
			}

			// Scroll the view if necessary
			if m.selected >= m.scrollOffset+m.visibleItemsCount() {
				m.scrollOffset++
			}
		case "up":
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

	// Display the search prompt and filter
	b.WriteString("Search (Press Ctrl+C to quit):\n")
	b.WriteString("Filter: " + m.filter + "\n")

	if len(m.filteredKeys) == 0 {
		b.WriteString("\nNo matches found.\n")
	} else {
		visibleItems := m.visibleItemsCount()

		// Determine the slice of items to display based on scroll offset
		start := m.scrollOffset
		end := start + visibleItems
		if end > len(m.filteredKeys) {
			end = len(m.filteredKeys)
		}

		// Render the options within the visible range
		for i := start; i < end; i++ {
			if i == m.selected {
				b.WriteString(fmt.Sprintf("▶ %s\n", m.filteredKeys[i].Label)) // Highlight the selected option
			} else {
				b.WriteString(fmt.Sprintf("  %s\n", m.filteredKeys[i].Label)) // Regular option
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
	m.filteredKeys = []SelectionOption{}

	for key, value := range m.options {
		// When the key is " ", compare and display using the value instead
		if key == " " {
			if strings.Contains(strings.ToLower(value), strings.ToLower(m.filter)) {
				m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: value, Key: key})
			}
		} else if strings.Contains(strings.ToLower(value), strings.ToLower(m.filter)) {
			m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: value, Key: key})
		}
	}

	// Sort the filtered options alphabetically
	sort.Slice(m.filteredKeys, func(i, j int) bool {
		return m.filteredKeys[i].Label < m.filteredKeys[j].Label
	})

	// Reset the selection to the "Add new anime" option if no matches are found
	if len(m.filteredKeys) == 0 {
		m.filteredKeys = append(m.filteredKeys, SelectionOption{
			Label: "Add new anime",
			Key:   "add_new",
		})
	}
	m.filteredKeys = append(m.filteredKeys, SelectionOption{
		Label: "Quit",
		Key:   "-1",
	})
}


func DynamicSelectPreview(options map[string]RofiSelectPreview, addnewoption bool) (SelectionOption, error) {
	// Pre-download first 14 images in background
	go preDownloadImages(options, 14)

	userCurdConfig := GetGlobalConfig()
	if userCurdConfig.StoragePath == "" {
		userCurdConfig.StoragePath = os.ExpandEnv("${HOME}/.local/share/curd")
	}

	// Prepare Rofi input with anime titles and their cached image paths
	var rofiInput strings.Builder
	for _, option := range options {
		// Download and get cache path for the image
		cachePath, err := downloadToCache(option.CoverImage)
		if err != nil {
			Log(fmt.Sprintf("Error caching image: %v", err), logFile)
			continue
		}
		
		// Format: "Title\x00icon\x1f/path/to/cached/image\n"
		// This tells Rofi to use the image as an icon for this entry
		rofiInput.WriteString(fmt.Sprintf("%s\x00icon\x1f%s\n", option.Title, cachePath))
	}

	// Add "Add new anime" and "Quit" options
	if addnewoption {
		rofiInput.WriteString("Add new anime\n")
	}
	rofiInput.WriteString("Quit\n")

	// Get the absolute path to the rasi config
	configPath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "selectanimepreview.rasi")
	
	// Create the command with explicit arguments
	args := []string{
		"-dmenu",
		"-theme", configPath,
		"-show-icons",
		"-p", "Select Anime",
		"-i",  // Case-insensitive matching
		"-no-custom",  // Disable custom input
	}
	
	// Create the command
	rofiCmd := exec.Command("rofi", args...)

	// Set up pipes for input/output
	rofiCmd.Stdin = strings.NewReader(rofiInput.String())
	var stdout, stderr bytes.Buffer
	rofiCmd.Stdout = &stdout
	rofiCmd.Stderr = &stderr
	
	// Run the command
	err := rofiCmd.Run()
	if err != nil {
		// Log both stdout and stderr for debugging
		Log(fmt.Sprintf("Rofi stderr: %s", stderr.String()), logFile)
		Log(fmt.Sprintf("Rofi stdout: %s", stdout.String()), logFile)
		return SelectionOption{}, fmt.Errorf("failed to execute rofi: %w", err)
	}
	
	selectedTitle := strings.TrimSpace(stdout.String())

	// Handle special cases
	switch selectedTitle {
	case "":
		return SelectionOption{}, fmt.Errorf("no selection made")
	case "Add new anime":
		return SelectionOption{Label: "Add new anime", Key: "add_new"}, nil
	case "Quit":
		return SelectionOption{Label: "Quit", Key: "-1"}, nil
	}

	// Find the selected anime in options
	for id, option := range options {
		if option.Title == selectedTitle {
			return SelectionOption{
				Label: option.Title,
				Key:   id,
			}, nil
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

	// Display the image with ueberzug
	cmd := exec.Command("ueberzug", "layer", "--silent", "add", "preview", "--path", cachePath)
	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to start image preview: %w", err)
	}
	time.Sleep(2 * time.Second) // Allow image to load for a moment
	return nil
}


func RofiSelect(options map[string]string, addanimeopt bool) (SelectionOption, error) {
	userCurdConfig := GetGlobalConfig()
	if userCurdConfig.StoragePath == "" {
		userCurdConfig.StoragePath = os.ExpandEnv("${HOME}/.local/share/curd")
	}

	// Create a slice to store the options in the order we want
	var optionsList []string
	for _, value := range options {
		optionsList = append(optionsList, value)
	}
	
	// Sort the options alphabetically
	sort.Strings(optionsList)
	
	// Add "Add new anime" and "Quit" options
	if addanimeopt {
		optionsList = append(optionsList, "Add new anime", "Quit")
	} else {
		optionsList = append(optionsList, "Quit")
	}
	
	// Join all options into a single string, separated by newlines
	optionsString := strings.Join(optionsList, "\n")
	
	// Prepare the Rofi command
	cmd := exec.Command("rofi", "-dmenu", "-theme", filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "selectanime.rasi"), "-i", "-p", "Select an anime")
	
	// Set up pipes for input and output
	cmd.Stdin = strings.NewReader(optionsString)
	var out bytes.Buffer
	cmd.Stdout = &out
	
	// Run the command
	err := cmd.Run()
	if err != nil {
		return SelectionOption{}, fmt.Errorf("failed to run Rofi: %v", err)
	}
	
	// Get the selected option
	selected := strings.TrimSpace(out.String())
	
	// Handle special cases
	switch selected {
	case "":
		return SelectionOption{}, fmt.Errorf("no selection made")
	case "Add new anime":
		return SelectionOption{Label: "Add new anime", Key: "add_new"}, nil
	case "Quit":
		return SelectionOption{Label: "Quit", Key: "-1"}, nil
	}
	
	// Find the key for the selected value
	for key, value := range options {
		if value == selected {
			return SelectionOption{Label: value, Key: key}, nil
		}
	}
	
	// If we get here, the selected option wasn't found in the original map
	return SelectionOption{}, fmt.Errorf("selected option not found in original list")
}

// DynamicSelect displays a simple selection prompt without extra features
func DynamicSelect(options map[string]string, addnewoption bool) (SelectionOption, error) {

	if GetGlobalConfig().RofiSelection {
		return RofiSelect(options, addnewoption)
	}

	model := &Model{
		options:      options,
		filteredKeys: make([]SelectionOption, 0),
	}

	model.filterOptions()
	p := tea.NewProgram(model)

	finalModel, err := p.Run()
	if err != nil {
		return SelectionOption{}, err
	}

	finalSelectionModel, ok := finalModel.(*Model)
	if !ok {
		return SelectionOption{}, fmt.Errorf("unexpected model type")
	}

	if finalSelectionModel.selected < len(finalSelectionModel.filteredKeys) {
		return finalSelectionModel.filteredKeys[finalSelectionModel.selected], nil
	}
	return SelectionOption{}, nil
}
