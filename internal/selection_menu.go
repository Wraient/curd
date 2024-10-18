package internal

import (
	"fmt"
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
	options      map[string]string // Now using a map for options
	filter       string
	filteredKeys []SelectionOption
	selected     int
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles user input and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Only filter options if a key press modifies the filter
	updateFilter := false

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				updateFilter = true // Mark for updating the filter
			}
		case "down":
			// Move cursor down within bounds
			if m.selected < len(m.filteredKeys)-1 {
				m.selected++
			}
		case "up":
			// Move cursor up within bounds
			if m.selected > 0 {
				m.selected--
			}
		case "enter":
			return m, tea.Quit // Or return the selected option
		default:
			if msg.String() != "" {
				m.filter += msg.String()
				updateFilter = true // Mark for updating the filter
			}
		}
	}

	// Only filter options if the filter was actually modified
	if updateFilter {
		m.filterOptions()
		// Reset selected index after filtering
		m.selected = 0
	}

	return m, nil
}


// View renders the UI
func (m Model) View() string {
	var b strings.Builder

	b.WriteString("Search (Press Ctrl+C to quit):\n\n")
	b.WriteString("Filter: " + m.filter + "\n\n")

	if len(m.filteredKeys) == 0 {
		b.WriteString("No matches found.\n")
	} else {
		for i, option := range m.filteredKeys {
			if i == m.selected {
				b.WriteString(fmt.Sprintf("▶ %s\n", option.Label))
			} else {
				b.WriteString(fmt.Sprintf("  %s\n", option.Label))
			}
		}
	}

	return b.String()
}

// filterOptions filters options based on the search term
func (m *Model) filterOptions() {
	m.filteredKeys = nil
	for key, value := range m.options {
		if strings.Contains(strings.ToLower(value), strings.ToLower(m.filter)) { // Check values instead of keys
			m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: value, Key: key}) // Set Label to value
		}
	}
}

// DynamicSelect displays a full-screen selection prompt with search
func DynamicSelect(options map[string]string) (SelectionOption, error) {
	ClearScreen()
	defer RestoreScreen()
	model := &Model{ // Use a pointer to Model
		options: options,
		filteredKeys: make([]SelectionOption, 0), // Initialize filteredKeys
	}

	// Populate filteredKeys initially
	model.filterOptions()

	// Create a new Program instance
	p := tea.NewProgram(model)

	// Run the program and capture the final model and any error
	finalModel, err := p.Run()
	if err != nil {
		return SelectionOption{}, err
	}

	// Type assert to get the specific model type
	finalSelectionModel, ok := finalModel.(*Model)
	if !ok {
		return SelectionOption{}, fmt.Errorf("unexpected model type")
	}

	// Return the selected option after quitting
	if finalSelectionModel.selected < len(finalSelectionModel.filteredKeys) {
		return finalSelectionModel.filteredKeys[finalSelectionModel.selected], nil
	}
	return SelectionOption{}, nil
}
