import curses
import re
import os

current_dir = os.path.dirname(os.path.abspath(__file__))

def load_links(filename):
    links = []
    with open(filename, 'r') as file:
        content = file.read()
        # Regex to match the name and URL pairs
        matches = re.findall(r'(\S+)\s*>\s*(https?://\S+)', content)
        links = [(name, url) for name, url in matches]
    return links

def display_links(stdscr, links):
    curses.curs_set(0)  # Hide the cursor
    stdscr.clear()
    h, w = stdscr.getmaxyx()
    max_height = h - 1
    start = 0
    selected_index = 0

    while True:
        stdscr.clear()
        for i in range(start, min(start + max_height, len(links))):
            if i == selected_index:
                stdscr.attron(curses.A_REVERSE)  # Highlight the selected item
            stdscr.addstr(i - start, 0, links[i][0])
            if i == selected_index:
                stdscr.attroff(curses.A_REVERSE)  # Remove highlight after the line
        stdscr.addstr(max_height, 0, "Use arrow keys to scroll, Enter to select, q to quit")
        stdscr.refresh()
        
        key = stdscr.getch()
        
        if key == curses.KEY_DOWN:
            if selected_index < len(links) - 1:
                selected_index += 1
                if selected_index >= start + max_height:
                    start += 1
        elif key == curses.KEY_UP:
            if selected_index > 0:
                selected_index -= 1
                if selected_index < start:
                    start -= 1
        elif key == curses.KEY_ENTER or key == 10:  # Enter key
            stdscr.clear()
            stdscr.addstr(0, 0, f"Selected URL: {links[selected_index][1]}")
            # print("done")
            with open(f"/tmp/curd/curd_link", "w") as temp:
                temp.write(links[selected_index][1])
            stdscr.refresh()
            # stdscr.getch()  # Wait for any key press to exit
            return links[selected_index][1]
            break
        elif key == ord('q'):
            break

def main(stdscr):
    filename = f"/tmp/curd/curd_links"  # Replace with your file name
    links = load_links(filename)
    print("printed", display_links(stdscr, links))
    print("done")

# print(display_links(stdscr,links))
if __name__ == "__main__":
    try:
        curses.wrapper(main)
    except:
        pass