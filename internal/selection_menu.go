package internal

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
				fmt.Println("Adding a new anime...")
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
}

// DynamicSelect displays a simple selection prompt without extra features
func DynamicSelect(options map[string]string) (SelectionOption, error) {
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
