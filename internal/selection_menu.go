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
	"reflect"
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
	isHomeMenu     bool // If true, ESC quits; if false, ESC goes back
}

type optionsRefreshedMsg struct {
	options []SelectionOption
}

type SelectionRefreshConfig struct {
	Updates      <-chan AnimeList
	BuildOptions func(AnimeList) []SelectionOption
}

type PreviewSelectionRefreshConfig struct {
	Updates      <-chan AnimeList
	BuildOptions func(AnimeList) map[string]RofiSelectPreview
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
	case optionsRefreshedMsg:
		m.replaceOptions(msg.options)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			// Return quit selection option directly
			return m, tea.Quit
		case "esc":
			// ESC: quit on home menu, go back on sub-menus
			if m.isHomeMenu {
				// Quit
				m.filteredKeys = []SelectionOption{{Key: "-1", Label: "Quit"}}
				m.selected = 0
			} else {
				// Go back
				m.filteredKeys = []SelectionOption{{Key: "-2", Label: "Back"}}
				m.selected = 0
			}
			return m, tea.Quit
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				updateFilter = true
			}
		case "down", "tab", "ctrl+n":
			// Move the selection cursor down
			if m.selected < len(m.filteredKeys)-1 {
				m.selected++
			}

			// Scroll the view if necessary
			if m.selected >= m.scrollOffset+m.visibleItemsCount() {
				m.scrollOffset++
			}
		case "up", "shift+tab", "ctrl+p":
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
				m.filteredKeys[m.selected] = SelectionOption{Label: "add_new", Key: "0"}
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

func (m *Model) replaceOptions(options []SelectionOption) {
	previousIndex := m.selected
	previousKey := ""
	previousLabel := ""

	if m.selected >= 0 && m.selected < len(m.filteredKeys) {
		previousKey = m.filteredKeys[m.selected].Key
		previousLabel = m.filteredKeys[m.selected].Label
	}

	m.allOptions = options
	m.filterOptions()

	if len(m.filteredKeys) == 0 {
		m.selected = 0
		m.scrollOffset = 0
		return
	}

	m.selected = findSelectionIndex(m.filteredKeys, previousKey, previousLabel, previousIndex)
	if m.selected < 0 {
		m.selected = 0
	}

	if m.selected < m.scrollOffset {
		m.scrollOffset = m.selected
	}

	visibleCount := m.visibleItemsCount()
	if visibleCount <= 0 {
		m.scrollOffset = 0
		return
	}

	if m.selected >= m.scrollOffset+visibleCount {
		m.scrollOffset = m.selected - visibleCount + 1
	}
	if m.scrollOffset < 0 {
		m.scrollOffset = 0
	}
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

	// Determine whether the current filter is a substring of the pinned labels.
	// When the filter matches "back" (e.g. "b", "ba", "bac", "back"), Back should
	// appear BEFORE "Add new anime" so it is easy to reach. Otherwise keep the
	// default order: Add new anime → Back → Quit.
	// All three pinned items are always shown regardless of filter text.
	filterLower := strings.ToLower(m.filter)
	backMatchesFilter := filterLower != "" && strings.Contains("back", filterLower)

	// If filter targets "back", pin it above Add new anime
	if !m.isHomeMenu && backMatchesFilter {
		m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: "Back", Key: "-2"})
	}

	// Add new anime is always shown (pinned)
	if m.addNewOption {

		m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: "Add new anime", Key: "add_new"})
	}

	// Back in its default position (after Add new anime) when filter doesn't target it
	if !m.isHomeMenu && !backMatchesFilter {
		m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: "Back", Key: "-2"})
	}

	// Quit is always last
	m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: "Quit", Key: "-1"})
}

func detectHomeMenu(options []SelectionOption) bool {
	for _, opt := range options {
		if opt.Key == "ALL" || opt.Key == "CURRENT" {
			return true
		}
	}
	return false
}

func findSelectionIndex(options []SelectionOption, previousKey string, previousLabel string, fallbackIndex int) int {
	if previousKey != "" {
		for idx, option := range options {
			if option.Key == previousKey {
				return idx
			}
		}
	}

	if previousLabel != "" {
		for idx, option := range options {
			if option.Label == previousLabel {
				return idx
			}
		}
	}

	if fallbackIndex >= 0 && fallbackIndex < len(options) {
		return fallbackIndex
	}

	if len(options) == 0 {
		return -1
	}

	return min(fallbackIndex, len(options)-1)
}

func sortHomeMenuOptions(options []SelectionOption) []SelectionOption {
	menuOrder := strings.Split(GetGlobalConfig().MenuOrder, ",")
	optMap := make(map[string]SelectionOption)
	for _, opt := range options {
		optMap[opt.Key] = opt
	}

	sorted := make([]SelectionOption, 0, len(options))
	for _, key := range menuOrder {
		if opt, exists := optMap[key]; exists {
			sorted = append(sorted, opt)
			delete(optMap, key)
		}
	}

	for _, opt := range options {
		if _, exists := optMap[opt.Key]; exists {
			sorted = append(sorted, opt)
			delete(optMap, opt.Key)
		}
	}

	return sorted
}

func previewOptionsToSortedSelection(options map[string]RofiSelectPreview) []SelectionOption {
	selectionOptions := make([]SelectionOption, 0, len(options))
	for id, opt := range options {
		selectionOptions = append(selectionOptions, SelectionOption{
			Label: opt.Title,
			Key:   id,
		})
	}

	sort.Slice(selectionOptions, func(i, j int) bool {
		return selectionOptions[i].Label < selectionOptions[j].Label
	})

	return selectionOptions
}

func DynamicSelectPreview(options map[string]RofiSelectPreview, addnewoption bool) (SelectionOption, error) {
	return DynamicSelectPreviewWithRefresh(options, addnewoption, nil)
}

func DynamicSelectPreviewWithRefresh(options map[string]RofiSelectPreview, addnewoption bool, refreshConfig *PreviewSelectionRefreshConfig) (SelectionOption, error) {
	go preDownloadImages(options, 14)

	userCurdConfig := GetGlobalConfig()
	if userCurdConfig.StoragePath == "" {
		userCurdConfig.StoragePath = os.ExpandEnv("${HOME}/.local/share/curd")
	}

	currentOptions := options

	for {
		var rofiInput strings.Builder
		selectionOptions := previewOptionsToSortedSelection(currentOptions)

		for _, opt := range selectionOptions {
			cachePath, err := downloadToCache(currentOptions[opt.Key].CoverImage)
			if err != nil {
				Log(fmt.Sprintf("Error caching image: %v", err))
				continue
			}
			rofiInput.WriteString(fmt.Sprintf("%s\x00icon\x1f%s\n", opt.Label, cachePath))
		}

		if addnewoption {

			rofiInput.WriteString("Add new anime\n")
		}
		rofiInput.WriteString("Back\n")
		rofiInput.WriteString("Quit\n")

		configPath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "selectanimepreview.rasi")
		cmd := exec.Command("rofi", "-dmenu", "-theme", configPath, "-show-icons", "-p", "Select Anime", "-i", "-no-custom")
		cmd.Stdin = strings.NewReader(rofiInput.String())
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if refreshConfig == nil || refreshConfig.Updates == nil {
			if err := cmd.Run(); err != nil {
				Log(fmt.Sprintf("Rofi stderr: %s", stderr.String()))
				Log(fmt.Sprintf("Rofi stdout: %s", stdout.String()))
				return SelectionOption{Key: "-2", Label: "Back"}, nil
			}
			return parsePreviewSelection(stdout.String(), selectionOptions)
		}

		if err := cmd.Start(); err != nil {
			return SelectionOption{}, fmt.Errorf("failed to run Rofi preview menu: %w", err)
		}

		waitCh := make(chan error, 1)
		go func() {
			waitCh <- cmd.Wait()
		}()

		restartMenu := false

		for !restartMenu {
			select {
			case err := <-waitCh:
				if err != nil {
					Log(fmt.Sprintf("Rofi stderr: %s", stderr.String()))
					Log(fmt.Sprintf("Rofi stdout: %s", stdout.String()))
					return SelectionOption{Key: "-2", Label: "Back"}, nil
				}
				return parsePreviewSelection(stdout.String(), selectionOptions)
			case updatedList, ok := <-refreshConfig.Updates:
				if !ok {
					refreshConfig = nil
					continue
				}

				updatedOptions := refreshConfig.BuildOptions(updatedList)
				if reflect.DeepEqual(currentOptions, updatedOptions) {
					continue
				}

				currentOptions = updatedOptions
				restartMenu = true

				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				<-waitCh
			}
		}
	}
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

func parsePreviewSelection(rawSelection string, selectionOptions []SelectionOption) (SelectionOption, error) {
	selected := strings.TrimSpace(rawSelection)

	switch selected {
	case "":
		return SelectionOption{Key: "-2", Label: "Back"}, nil
	case "Add new anime":
		return SelectionOption{Label: "Add new anime", Key: "add_new"}, nil

	case "Back":
		return SelectionOption{Label: "Back", Key: "-2"}, nil
	case "Quit":
		return SelectionOption{Label: "Quit", Key: "-1"}, nil
	}

	for _, opt := range selectionOptions {
		if opt.Label == selected {
			return opt, nil
		}
	}

	return SelectionOption{}, fmt.Errorf("selection not found in options")
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

func RofiSelect(options []SelectionOption, isHomeMenu bool) (SelectionOption, error) {
	return RofiSelectWithRefresh(options, isHomeMenu, nil)
}

func RofiSelectWithRefresh(options []SelectionOption, isHomeMenu bool, refreshConfig *SelectionRefreshConfig) (SelectionOption, error) {
	userCurdConfig := GetGlobalConfig()
	if userCurdConfig.StoragePath == "" {
		userCurdConfig.StoragePath = os.ExpandEnv("${HOME}/.local/share/curd")
	}

	currentOptions := options

	for {
		optionsString := buildRofiOptionsString(currentOptions, isHomeMenu)
		configPath := filepath.Join(os.ExpandEnv(userCurdConfig.StoragePath), "selectanime.rasi")
		cmd := exec.Command("rofi", "-dmenu", "-theme", configPath, "-i", "-p", "Select")
		cmd.Stdin = strings.NewReader(optionsString)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if refreshConfig == nil || refreshConfig.Updates == nil {
			err := cmd.Run()
			return parseRofiSelection(err, stdout.String(), currentOptions, isHomeMenu)
		}

		if err := cmd.Start(); err != nil {
			return SelectionOption{}, fmt.Errorf("failed to run Rofi: %w", err)
		}

		waitCh := make(chan error, 1)
		go func() {
			waitCh <- cmd.Wait()
		}()

		restartMenu := false

		for !restartMenu {
			select {
			case err := <-waitCh:
				if err != nil {
					Log(fmt.Sprintf("Rofi stderr: %s", stderr.String()))
				}
				return parseRofiSelection(err, stdout.String(), currentOptions, isHomeMenu)
			case updatedList, ok := <-refreshConfig.Updates:
				if !ok {
					refreshConfig = nil
					continue
				}

				updatedOptions := refreshConfig.BuildOptions(updatedList)
				if reflect.DeepEqual(currentOptions, updatedOptions) {
					continue
				}

				currentOptions = updatedOptions
				restartMenu = true

				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				<-waitCh
			}
		}
	}
}

func DynamicSelectFromSlice(options []SelectionOption) (SelectionOption, error) {
	return dynamicSelectInternal(options, nil)
}

// DynamicSelect displays a simple selection prompt without extra features
func DynamicSelect(options []SelectionOption) (SelectionOption, error) {
	return dynamicSelectInternal(options, nil)
}

func DynamicSelectWithRefresh(options []SelectionOption, refreshConfig *SelectionRefreshConfig) (SelectionOption, error) {
	return dynamicSelectInternal(options, refreshConfig)
}

func dynamicSelectInternal(options []SelectionOption, refreshConfig *SelectionRefreshConfig) (SelectionOption, error) {
	isHomeMenu := detectHomeMenu(options)

	if isHomeMenu {
		options = sortHomeMenuOptions(options)
	}

	for _, opt := range options {
		if strings.Contains(opt.Label, "Bleach (366 episodes) [animepahe]") {
			return opt, nil
		}
	}

	if GetGlobalConfig().RofiSelection {
		return RofiSelectWithRefresh(options, isHomeMenu, refreshConfig)
	}

	// Separate out the "add_new" sentinel so it is never sorted alphabetically.
	// The addNewOption flag causes filterOptions() to append it after the sort.
	hasAddNew := false
	cleanOptions := make([]SelectionOption, 0, len(options))
	for _, opt := range options {
		if opt.Key == "add_new" {
			hasAddNew = true
		} else {
			cleanOptions = append(cleanOptions, opt)
		}
	}

	model := &Model{
		allOptions:   cleanOptions,
		isHomeMenu:   isHomeMenu,
		addNewOption: hasAddNew,
	}
	model.filterOptions()

	p := tea.NewProgram(model)
	stopRefresh := make(chan struct{})

	if refreshConfig != nil && refreshConfig.Updates != nil {
		go func(lastOptions []SelectionOption) {
			currentOptions := lastOptions
			for {
				select {
				case <-stopRefresh:
					return
				case updatedList, ok := <-refreshConfig.Updates:
					if !ok {
						return
					}

					updatedOptions := refreshConfig.BuildOptions(updatedList)
					if reflect.DeepEqual(currentOptions, updatedOptions) {
						continue
					}

					currentOptions = updatedOptions
					p.Send(optionsRefreshedMsg{options: updatedOptions})
				}
			}
		}(append([]SelectionOption(nil), options...))
	}

	finalModel, err := p.Run()
	close(stopRefresh)
	if err != nil {
		return SelectionOption{}, err
	}

	fmt.Print("\033[?25h")
	fmt.Print("\033[?7h")

	finalSelectionModel, ok := finalModel.(*Model)
	if !ok {
		return SelectionOption{}, fmt.Errorf("unexpected model type")
	}

	if finalSelectionModel.selected < len(finalSelectionModel.filteredKeys) {
		return finalSelectionModel.filteredKeys[finalSelectionModel.selected], nil
	}
	return SelectionOption{}, nil
}

func buildRofiOptionsString(options []SelectionOption, isHomeMenu bool) string {
	optionsList := make([]string, 0, len(options)+2)
	for _, opt := range options {
		optionsList = append(optionsList, opt.Label)
	}

	if !isHomeMenu {
		optionsList = append(optionsList, "Back")
	}
	optionsList = append(optionsList, "Quit")

	return strings.Join(optionsList, "\n")
}

func parseRofiSelection(err error, rawSelection string, options []SelectionOption, isHomeMenu bool) (SelectionOption, error) {
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 1 {
			if isHomeMenu {
				return SelectionOption{Key: "-1", Label: "Quit"}, nil
			}
			return SelectionOption{Key: "-2", Label: "Back"}, nil
		}
		return SelectionOption{}, fmt.Errorf("failed to run Rofi: %v", err)
	}

	selected := strings.TrimSpace(rawSelection)
	switch selected {
	case "":
		if isHomeMenu {
			return SelectionOption{Key: "-1", Label: "Quit"}, nil
		}
		return SelectionOption{Key: "-2", Label: "Back"}, nil
	case "Back":
		return SelectionOption{Label: "Back", Key: "-2"}, nil
	case "Quit":
		return SelectionOption{Label: "Quit", Key: "-1"}, nil
	}

	for _, opt := range options {
		if opt.Label == selected {
			return opt, nil
		}
	}

	return SelectionOption{}, fmt.Errorf("selected option not found in original list")
}
