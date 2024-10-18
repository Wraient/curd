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
	terminalHeight int // New field to hold the terminal height
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles user input and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle terminal resize messages
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.terminalHeight = wsm.Height // Get the terminal height
	}

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
            // If "Add new anime" is selected, handle it here
            if m.filteredKeys[m.selected].Key == "add_new" {
                // Implement your logic for adding a new anime
                fmt.Println("Adding a new anime...") // Temporary print statement
                return m, tea.Quit
            }
            return m, tea.Quit // Or return the selected option for other cases
        default:
			if len(msg.String()) == 1 && msg.String() >= " " && msg.String() <= "~" {
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
        // Determine how many entries can fit in the terminal
        maxEntries := m.terminalHeight - 4 // Adjust for header and padding
        if maxEntries < 1 {
            maxEntries = 1 // At least show one option
        }

        // Limit the number of displayed entries
        start := m.selected
        if start >= len(m.filteredKeys) {
            start = len(m.filteredKeys) - 1
        }
        end := start + maxEntries
        if end > len(m.filteredKeys) {
            end = len(m.filteredKeys)
        }
        if start > end {
            start = end
        }

        // Display the filtered keys with selection indicator
        for i := start; i < end; i++ {
            if i == m.selected {
                b.WriteString(fmt.Sprintf("▶ %s\n", m.filteredKeys[i].Label))
            } else {
                b.WriteString(fmt.Sprintf("  %s\n", m.filteredKeys[i].Label))
            }
        }
    }

    return b.String()
}


// filterOptions filters options based on the search term
func (m *Model) filterOptions() {
    // Clear the filteredKeys slice before appending new matches
    m.filteredKeys = []SelectionOption{}

    for key, value := range m.options {
        if strings.Contains(strings.ToLower(value), strings.ToLower(m.filter)) {
            m.filteredKeys = append(m.filteredKeys, SelectionOption{Label: value, Key: key})
        }
    }

    // If no matches were found, add the "Add new anime" option
    if len(m.filteredKeys) == 0 {
        m.filteredKeys = append(m.filteredKeys, SelectionOption{
            Label: "Add new anime",
            Key:   "add_new", // Special key to identify this option
        })
    }
}


// DynamicSelect displays a full-screen selection prompt with search
func DynamicSelect(options map[string]string) (SelectionOption, error) {
	// ClearScreen()
	// defer RestoreScreen()
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
