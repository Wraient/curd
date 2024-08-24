
import curses

# Sample dictionary of anime names and their IDs

def load_anime_data(filename):
    anime_list = {}
    with open(filename, "r") as file:
        data = file.read().strip()
        entries = data.split("NEWLINEFROMHERE")
        for entry in entries:
            if entry.strip():
                parts = entry.split(maxsplit=1)
                if len(parts) == 2:
                    anime_id, anime_name = parts
                    anime_list[anime_name] = anime_id
    return anime_list
anime_dict = load_anime_data("scripts/tmp/anime_list")

def main(stdscr):
    # Clear screen
    curses.curs_set(1)
    stdscr.clear()
    stdscr.refresh()

    # Initial values
    current_input = ""
    filtered_list = list(anime_dict.keys())
    selected_index = 0
    scroll_position = 0  # Position to start displaying the list from

    try:
        while True:
            stdscr.clear()

            # Get terminal dimensions
            height, width = stdscr.getmaxyx()
            list_height = height - 2  # Leave space for input and status lines

            # Handle edge case where terminal window is too small
            if height < 5 or width < len("Search Anime: ") + len(current_input):
                stdscr.addstr(0, 0, "Terminal window too small!")
                stdscr.refresh()
                continue

            # Display search prompt
            stdscr.addstr(0, 0, "Search Anime: " + current_input)

            # Calculate scrolling parameters
            if selected_index < scroll_position:
                scroll_position = selected_index
            elif selected_index >= scroll_position + list_height:
                scroll_position = selected_index - list_height + 1

            # Display filtered results with highlighting and scrolling
            for idx in range(scroll_position, min(len(filtered_list), scroll_position + list_height)):
                anime = filtered_list[idx]
                display_idx = idx - scroll_position + 1
                if idx == selected_index:
                    stdscr.addstr(display_idx, 0, anime, curses.A_REVERSE)
                else:
                    stdscr.addstr(display_idx, 0, anime)

            # Get user input
            key = stdscr.getch()

            # Handle key inputs
            if key in [curses.KEY_BACKSPACE, 127, 8]:
                current_input = current_input[:-1]
                selected_index = 0
            elif key == curses.KEY_DOWN:
                if selected_index < len(filtered_list) - 1:
                    selected_index += 1
            elif key == curses.KEY_UP:
                if selected_index > 0:
                    selected_index -= 1
            elif key == curses.KEY_ENTER or key in [10, 13]:  # Enter key
                if filtered_list:
                    selected_anime = filtered_list[selected_index]
                    selected_id = anime_dict[selected_anime]
                    break
            elif key == ord('q'):  # Exit if 'q' is pressed
                return
            else:
                try:
                    current_input += chr(key)
                    selected_index = 0
                except:
                    pass

            # Filter the anime list based on current input
            filtered_list = [anime for anime in anime_dict.keys() if current_input.lower() in anime.lower()]

            # Ensure selected index is within bounds after filtering
            if selected_index >= len(filtered_list):
                selected_index = len(filtered_list) - 1

            # Refresh screen
            stdscr.refresh()

    except Exception as e:
        # If an exception occurs, print it after exiting curses
        curses.endwin()
        print(f"An error occurred: {e}")
    finally:
        # Ensure curses always exits cleanly
        curses.endwin()

    # Print selected anime and its ID after exiting curses mode
    print(f"Selected Anime: {selected_anime}, ID: {selected_id}")
    with open("scripts/tmp/id", "w") as id:
        id.write(selected_id)

# Initialize curses
try:
    curses.wrapper(main)
except:
    pass
