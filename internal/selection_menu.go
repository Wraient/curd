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
	filterActive   bool // when VimKeys is on: true after "/" enters search mode
	filteredKeys   []SelectionOption
	allOptions     []SelectionOption
	selected       int
	terminalWidth  int
	terminalHeight int
	scrollOffset   int
	addNewOption   bool
	isHomeMenu     bool // If true, ESC quits; if false, ESC goes back
	preserveOrder  bool // skip alphabetical sort (action menus with a fixed priority)
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

	newEpisodeItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#4CAF50")) // Green

	rofiNewEpisodeColor = "#4CAF50"
)

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return nil
}

func (m *Model) moveSelectionDown() {
	if m.selected < len(m.filteredKeys)-1 {
		m.selected++
	}
	if m.selected >= m.scrollOffset+m.visibleItemsCount() {
		m.scrollOffset++
	}
}

func (m *Model) moveSelectionUp() {
	if m.selected > 0 {
		m.selected--
	}
	if m.selected < m.scrollOffset {
		m.scrollOffset--
	}
}

func (m *Model) confirmSelection() tea.Cmd {
	if len(m.filteredKeys) == 0 {
		return nil
	}
	if m.filteredKeys[m.selected].Key == "add_new" {
		CurdOut("Adding a new anime...")
		m.filteredKeys[m.selected] = SelectionOption{Label: "add_new", Key: "0"}
	}
	return tea.Quit
}

func (m *Model) exitMenu() tea.Cmd {
	if m.isHomeMenu {
		m.filteredKeys = []SelectionOption{{Key: "-1", Label: "Quit"}}
	} else {
		m.filteredKeys = []SelectionOption{{Key: "-2", Label: "Back"}}
	}
	m.selected = 0
	return tea.Quit
}

// Update handles user input and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle terminal resize messages
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.terminalWidth = wsm.Width
		m.terminalHeight = wsm.Height
	}

	updateFilter := false
	vimKeys := VimKeysEnabled(nil)

	switch msg := msg.(type) {
	case optionsRefreshedMsg:
		m.replaceOptions(msg.options)
		return m, nil
	case tea.KeyMsg:
		key := msg.String()

		switch key {
		case "ctrl+c":
			m.filteredKeys = []SelectionOption{{Key: "-1", Label: "Quit"}}
			m.selected = 0
			return m, tea.Quit
		}

		// --- Vim-enabled selection: normal mode vs search mode ---
		// Normal: hjkl/arrows move. Search (after /): like vim's / — every
		// printable key including hjkl is part of the query; arrows/tab still move.
		if vimKeys {
			if m.filterActive {
				switch key {
				case "esc":
					// Leave search mode but keep the current filter applied.
					m.filterActive = false
					return m, nil
				case "enter":
					return m, m.confirmSelection()
				case "backspace":
					if len(m.filter) > 0 {
						m.filter = m.filter[:len(m.filter)-1]
						updateFilter = true
					}
				case "down", "tab", "ctrl+n":
					m.moveSelectionDown()
				case "up", "shift+tab", "ctrl+p":
					m.moveSelectionUp()
				// left/right: optional result navigation without stealing hjkl
				case "left":
					m.moveSelectionUp()
				case "right":
					m.moveSelectionDown()
				default:
					// hjkl and all other printables go into the search query.
					if len(key) == 1 && key >= " " && key <= "~" {
						m.filter += key
						updateFilter = true
					}
				}
				break
			}

			// Normal mode: motions only; "/" or "?" starts search.
			switch key {
			case "/", "?":
				m.filterActive = true
				return m, nil
			case "esc":
				return m, m.exitMenu()
			case "enter":
				return m, m.confirmSelection()
			case "down", "j", "tab", "ctrl+n", "l", "right":
				m.moveSelectionDown()
			case "up", "k", "shift+tab", "ctrl+p", "h", "left":
				m.moveSelectionUp()
			case "backspace":
				// Ignore — filter is only edited in search mode.
			default:
				// Do not type into filter in normal mode.
			}
			break
		}

		// --- Legacy behavior: every printable key filters immediately ---
		switch key {
		case "esc":
			return m, m.exitMenu()
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				updateFilter = true
			}
		case "down", "tab", "ctrl+n":
			m.moveSelectionDown()
		case "up", "shift+tab", "ctrl+p":
			m.moveSelectionUp()
		case "enter":
			return m, m.confirmSelection()
		default:
			if len(key) == 1 && key >= " " && key <= "~" {
				m.filter += key
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
	if VimKeysEnabled(nil) {
		if m.filterActive {
			b.WriteString(titleStyle.Render("Search") + " (type query · arrows move · Enter: select · Esc: normal):\n")
			b.WriteString(filterLabelStyle.Render("/") +
				filterTextStyle.Render(m.filter+"▌") + "\n\n")
		} else {
			b.WriteString(titleStyle.Render("Select") + " (hjkl/arrows · / search · Enter · Esc):\n")
			if m.filter != "" {
				b.WriteString(filterLabelStyle.Render("Filter: ") +
					filterTextStyle.Render(m.filter) + "\n\n")
			} else {
				b.WriteString(quitHintStyle.Render("Press / to search") + "\n\n")
			}
		}
	} else {
		b.WriteString(titleStyle.Render("Search") + " (Press " +
			quitHintStyle.Render("Ctrl+C") + " to quit):\n")
		b.WriteString(filterLabelStyle.Render("Filter: ") +
			filterTextStyle.Render(m.filter) + "\n\n")
	}

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
			label := m.filteredKeys[i].Label

			if i == m.selected {
				if m.filteredKeys[i].HasNewEpisodes {
					b.WriteString(newEpisodeItemStyle.Render(" [NEW]") + selectedItemStyle.Render(label) + "\n")
				} else {
					b.WriteString(selectedItemStyle.Render(label) + "\n")
				}
			} else if m.filteredKeys[i].HasNewEpisodes {
				b.WriteString(newEpisodeItemStyle.Render(" [NEW]") + regularItemStyle.Render(label) + "\n")
			} else {
				b.WriteString(regularItemStyle.Render(label) + "\n")
			}
		}
	}

	return b.String()
}

// visibleItemsCount calculates how many options fit in the terminal
func (m Model) visibleItemsCount() int {
	// Leave space for the filter and other UI elements
	count := m.terminalHeight - 4 // Adjust this number based on your terminal layout
	if count < 1 {
		return 1
	}
	return count
}

func displayLabel(opt SelectionOption) string {
	if opt.HasNewEpisodes {
		return "[NEW]" + opt.Label
	}
	return opt.Label
}

// filterOptions filters and sorts options based on the search term
func (m *Model) filterOptions() {
	m.filteredKeys = nil
	for _, opt := range m.allOptions {
		// Small function to also consider new episode from list
		if strings.Contains(strings.ToLower(displayLabel(opt)), strings.ToLower(m.filter)) {
			m.filteredKeys = append(m.filteredKeys, opt)
		}
	}

	// Sort alphabetically unless this is a home menu or an ordered action list.
	isMenu := m.preserveOrder
	if !isMenu {
		for _, opt := range m.allOptions {
			if opt.Key == "ALL" || opt.Key == "CURRENT" {
				isMenu = true
				break
			}
		}
	}

	if !isMenu {
		sort.Slice(m.filteredKeys, func(i, j int) bool {
			return m.filteredKeys[i].Label < m.filteredKeys[j].Label
		})
	}

	// Pin Back / Add new / Quit only when the filter is empty or matches their labels.
	// Previously Quit/Back were always forced visible, so "/quit" still showed Back first
	// and Enter could select the wrong row.
	filterLower := strings.ToLower(strings.TrimSpace(m.filter))
	pinMatches := func(label string) bool {
		if filterLower == "" {
			return true
		}
		return strings.Contains(strings.ToLower(label), filterLower)
	}
	backMatchesFilter := filterLower != "" && strings.Contains("back", filterLower)

	// If filter targets "back", pin it above Add new anime
	if !m.isHomeMenu && backMatchesFilter {
		m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: "Back", Key: "-2"})
	}

	// Add new anime when unfiltered or filter matches
	if m.addNewOption && pinMatches("Add new anime") {
		m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: "Add new anime", Key: "add_new"})
	}

	// Back in its default position when unfiltered (or filter matches "back" handled above)
	if !m.isHomeMenu && !backMatchesFilter && pinMatches("Back") {
		m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: "Back", Key: "-2"})
	}

	// Quit last — only when unfiltered or filter matches "quit"
	if pinMatches("Quit") {
		m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: "Quit", Key: "-1"})
	}
}

func detectHomeMenu(options []SelectionOption) bool {
	for _, opt := range options {
		if opt.Key == "ALL" || opt.Key == "CURRENT" {
			return true
		}
	}
	return false
}

// SelectionMeansQuit reports whether the user chose Quit (by key or label).
func SelectionMeansQuit(opt SelectionOption) bool {
	if opt.Key == "-1" {
		return true
	}
	label := strings.TrimSpace(opt.Label)
	return strings.EqualFold(label, "Quit") || strings.EqualFold(label, "quit")
}

// SelectionMeansBack reports whether the user chose Back / dismiss (by key or label).
func SelectionMeansBack(opt SelectionOption) bool {
	if opt.Key == "-2" || strings.EqualFold(opt.Key, "back") {
		return true
	}
	label := strings.ToLower(strings.TrimSpace(opt.Label))
	return label == "back" || label == "back to menu" || label == "back to list"
}

// NormalizeSelectionKey forces Quit/Back labels onto the canonical keys used by callers.
func NormalizeSelectionKey(opt SelectionOption) SelectionOption {
	if SelectionMeansQuit(opt) {
		return SelectionOption{Key: "-1", Label: "Quit"}
	}
	if SelectionMeansBack(opt) {
		return SelectionOption{Key: "-2", Label: "Back"}
	}
	return opt
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
	config := GetGlobalConfig()
	if config == nil || strings.TrimSpace(config.MenuOrder) == "" {
		return options
	}

	menuOrder := strings.Split(config.MenuOrder, ",")
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
			Label:          opt.Title,
			Key:            id,
			HasNewEpisodes: opt.HasNewEpisodes,
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

	// Removed boilerplate check

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
			label := opt.Label
			if opt.HasNewEpisodes {
				label = fmt.Sprintf("<span foreground=\"%s\">[NEW]</span> %s ", rofiNewEpisodeColor, opt.Label)
			}
			rofiInput.WriteString(fmt.Sprintf("%s\x00icon\x1f%s\n", label, cachePath))
		}

		if addnewoption {
			rofiInput.WriteString("Add new anime\n")
		}
		rofiInput.WriteString("Back\n")
		rofiInput.WriteString("Quit\n")

		configPath := filepath.Join(GetStoragePath(), "selectanimepreview.rasi")
		// NOTE: Need `-markup-rows` to enable pango
		cmd := exec.Command("rofi", "-dmenu", "-theme", configPath, "-show-icons", "-markup-rows", "-p", "Select Anime", "-i", "-no-custom")
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
	selected = strings.TrimSpace(pangoStrip.ReplaceAllString(selected, ""))
	selected = strings.TrimPrefix(selected, "[NEW] ")
	selected = strings.TrimSpace(selected)

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
	if strings.TrimSpace(imageURL) == "" {
		return "", fmt.Errorf("image URL is empty")
	}

	cacheDir := os.ExpandEnv("${HOME}/.cache/curd/images")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Create a hash of the URL to use as filename
	filename := fmt.Sprintf("%x.jpg", md5.Sum([]byte(imageURL)))
	cachePath := filepath.Join(cacheDir, filename)

	// Check if file already exists in cache
	if info, err := os.Stat(cachePath); err == nil {
		if info.Size() > 0 {
			return cachePath, nil
		}
		_ = os.Remove(cachePath)
	}

	// Download the image
	resp, err := sharedHTTPClient.Get(imageURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download image: status %d", resp.StatusCode)
	}

	file, err := os.Create(cachePath)
	if err != nil {
		return "", err
	}

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		file.Close()
		os.Remove(cachePath) // Clean up on error
		return "", err
	}
	if err := file.Close(); err != nil {
		os.Remove(cachePath)
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

// RofiSelectWithMessage shows a Rofi dmenu with an optional -mesg banner (for
// release notes, error diagnosis, etc.) so callers do not need notify-send spam.
func RofiSelectWithMessage(options []SelectionOption, isHomeMenu bool, prompt, message string) (SelectionOption, error) {
	return rofiSelectInternal(options, isHomeMenu, nil, prompt, message)
}

func RofiSelectWithRefresh(options []SelectionOption, isHomeMenu bool, refreshConfig *SelectionRefreshConfig) (SelectionOption, error) {
	return rofiSelectInternal(options, isHomeMenu, refreshConfig, "Select", "")
}

func rofiSelectInternal(options []SelectionOption, isHomeMenu bool, refreshConfig *SelectionRefreshConfig, prompt, message string) (SelectionOption, error) {
	currentOptions := options
	if strings.TrimSpace(prompt) == "" {
		prompt = "Select"
	}

	for {
		optionsString := buildRofiOptionsString(currentOptions, isHomeMenu)
		configPath := filepath.Join(GetStoragePath(), "selectanime.rasi")
		args := []string{"-dmenu", "-theme", configPath, "-i", "-markup", "-markup-rows", "-p", prompt}
		if msg := strings.TrimSpace(message); msg != "" {
			args = append(args, "-mesg", msg)
		}
		cmd := exec.Command("rofi", args...)
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
	return dynamicSelectInternal(options, nil, false)
}

// DynamicSelect displays a simple selection prompt without extra features
func DynamicSelect(options []SelectionOption) (SelectionOption, error) {
	return dynamicSelectInternal(options, nil, false)
}

// DynamicSelectPreserveOrder is like DynamicSelect but keeps the caller's option order
// (no alphabetical sort). Use for action menus where the first item is the primary action.
func DynamicSelectPreserveOrder(options []SelectionOption) (SelectionOption, error) {
	return dynamicSelectInternal(options, nil, true)
}

var promptSelect = DynamicSelect

// promptSelectOrdered is used for action menus where option order matters.
// Tests may replace this the same way as promptSelect.
var promptSelectOrdered = DynamicSelectPreserveOrder

func DynamicSelectWithRefresh(options []SelectionOption, refreshConfig *SelectionRefreshConfig) (SelectionOption, error) {
	return dynamicSelectInternal(options, refreshConfig, false)
}

func dynamicSelectInternal(options []SelectionOption, refreshConfig *SelectionRefreshConfig, preserveOrder bool) (SelectionOption, error) {
	isHomeMenu := detectHomeMenu(options)

	if isHomeMenu {
		options = sortHomeMenuOptions(options)
	}

	if config := GetGlobalConfig(); config != nil && config.RofiSelection {
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
		allOptions:    cleanOptions,
		isHomeMenu:    isHomeMenu,
		addNewOption:  hasAddNew,
		preserveOrder: preserveOrder,
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

	// Bubbletea may leave the terminal in raw mode; always reset cursor state.
	fmt.Print("\033[?25h")
	fmt.Print("\033[?7h")
	if err != nil {
		RestoreScreen()
		return SelectionOption{}, err
	}

	finalSelectionModel, ok := finalModel.(*Model)
	if !ok {
		return SelectionOption{}, fmt.Errorf("unexpected model type")
	}

	if finalSelectionModel.selected >= 0 && finalSelectionModel.selected < len(finalSelectionModel.filteredKeys) {
		return NormalizeSelectionKey(finalSelectionModel.filteredKeys[finalSelectionModel.selected]), nil
	}
	// Empty list / out of range — treat as cancel (Back for submenus, Quit on home).
	if finalSelectionModel.isHomeMenu {
		return SelectionOption{Key: "-1", Label: "Quit"}, nil
	}
	return SelectionOption{Key: "-2", Label: "Back"}, nil
}

func buildRofiOptionsString(options []SelectionOption, isHomeMenu bool) string {
	optionsList := make([]string, 0, len(options)+2)
	for _, opt := range options {
		if opt.HasNewEpisodes {
			optionsList = append(optionsList, fmt.Sprintf("<span foreground=\"%s\">[NEW]</span> %s", rofiNewEpisodeColor, opt.Label))
		} else {
			optionsList = append(optionsList, opt.Label)
		}
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
	// strip accidental pango noise if a theme echoes it.
	selected = strings.TrimSpace(pangoStrip.ReplaceAllString(
		ansiStrip.ReplaceAllString(selected, ""), "",
	))
	selected = strings.TrimPrefix(selected, "[NEW] ")
	selected = strings.TrimSpace(selected)
	switch {
	case selected == "":
		if isHomeMenu {
			return SelectionOption{Key: "-1", Label: "Quit"}, nil
		}
		return SelectionOption{Key: "-2", Label: "Back"}, nil
	case strings.EqualFold(selected, "Back"), strings.EqualFold(selected, "Back to menu"), strings.EqualFold(selected, "Back to list"):
		return SelectionOption{Label: "Back", Key: "-2"}, nil
	case strings.EqualFold(selected, "Quit"):
		return SelectionOption{Label: "Quit", Key: "-1"}, nil
	}

	for _, opt := range options {
		if opt.Label == selected || strings.EqualFold(opt.Label, selected) {
			return NormalizeSelectionKey(opt), nil
		}
		// Match when emoji/spacing differs slightly (e.g. double-space after emoji).
		if strings.EqualFold(strings.Join(strings.Fields(opt.Label), " "), strings.Join(strings.Fields(selected), " ")) {
			return NormalizeSelectionKey(opt), nil
		}
	}

	return SelectionOption{}, fmt.Errorf("selected option not found in original list")
}
