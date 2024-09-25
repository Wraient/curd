#!/bin/python3
# if you are trying to read this code, god help you.
# this is how my implementation of the ani-cli code to get the anime works

# search (anime_list.sh) -> tmp/anime_list 
# anime_id -> episode_list.sh
# episode_number -> episdoe_url.sh -> tmp/links 

from pathlib import Path
import re
import os
import json
import random
import subprocess
import time
import curses
import argparse
import traceback
import requests
import csv
import math
import socket
# pypresence is also imported later if user enabled it in the config

discord_client_id = "1287457464148820089"

# ----------------------------------------------- AniSkip Functions ----------------------------------------

def get_aniskip_data(anime_id, episode):
    base_url = "https://api.aniskip.com/v1/skip-times"
    url = f"{base_url}/{anime_id}/{episode}?types=op&types=ed"
    
    try:
        response = requests.get(url)
        response.raise_for_status()  # Raise an exception for bad status codes
        return response.text
    except requests.RequestException as e:
        print(f"Error fetching data from AniSkip API: {e}")
        return None

def round_time(time_value, precision=0):
    """
    Round a time value to a specified precision.
    precision=0 rounds to the nearest second.
    precision=1 rounds to the nearest tenth of a second, etc.
    """
    multiplier = 10 ** precision
    return math.floor(time_value * multiplier + 0.5) / multiplier

def parse_aniskip_response(response_text, time_precision=1):
    if response_text is None:
        return None
    
    data = json.loads(response_text)
    
    if not data['found']:
        return None
    
    skip_intervals = {}
    op = data['results'][0]['interval']
    # print(op)
    skip_intervals['op'] = {'start_time': round_time(op['start_time'], time_precision),
                            'end_time': round_time(op['end_time'], time_precision),
    }
    ed = data['results'][-1]['interval']
    skip_intervals['ed'] = {'start_time': round_time(ed['start_time'], time_precision),
                            'end_time': round_time(ed['end_time'], time_precision),
    }

    return skip_intervals
    
def get_and_parse_aniskip_data(anime_id, episode, time_precision=1):
    response_text = get_aniskip_data(anime_id, episode)
    return parse_aniskip_response(response_text, time_precision)

# Example usage
# anime_id = 28223
# episode = 10

# skip_intervals = get_and_parse_aniskip_data(anime_id, episode)

# print(skip_intervals)

# exit(0)

# if skip_intervals:
#     for interval in skip_intervals:
#         print(f"{interval['skip_type'].upper()} - Start: {interval['start_time']:.2f}, End: {interval['end_time']:.2f}")
# else:
#     print("No skip intervals found.")


# ----------------------------------------------- AniList Functions ----------------------------------------

def search_anime_anilist(query, token):
    url = "https://graphql.anilist.co"

    query_string = '''
    query ($search: String) {
      Page(page: 1, perPage: 10) {
        media(search: $search, type: ANIME) {
          id
          title {
            romaji
            english
            native
          }
        }
      }
    }
    '''

    variables = {
        'search': query
    }

    headers = {
        'Authorization': f'Bearer {token}',
        'Content-Type': 'application/json'
    }

    response = requests.post(url, json={'query': query_string, 'variables': variables}, headers=headers)
    
    if response.status_code == 200:
        anime_list = response.json()['data']['Page']['media']
        anime_dict = {}
        for anime in anime_list:
            if anime['title']['english']==None:
                anime_dict[anime['title']['romaji']] = anime['id']
            else:
                anime_dict[anime['title']['english']] = anime['id']
          
        return anime_dict
    else:
        print(f"Failed to search for anime. Status Code: {response.status_code}, Response: {response.text}")
        return {}

def get_anilist_user_id(token):
    url = "https://graphql.anilist.co"
    query = '''
    query {
        Viewer {
            id
            name
        }
    }
    '''
    
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Accept": "application/json"
    }
    
    response = requests.post(url, json={"query": query}, headers=headers)
    
    if response.status_code == 200:
        data = response.json()
        user_id = data['data']['Viewer']['id']
        user_name = data['data']['Viewer']['name']
        return user_id, user_name
    else:
        raise Exception(f"Error: {response.status_code}, {response.text}")

def add_anime_to_watching_list(anime_id: int, token: str):
    url = "https://graphql.anilist.co"

    mutation = '''
    mutation ($mediaId: Int) {
      SaveMediaListEntry (mediaId: $mediaId, status: CURRENT) {
        id
        status
      }
    }
    '''

    variables = {
        'mediaId': anime_id
    }

    headers = {
        'Authorization': f'Bearer {token}',
        'Content-Type': 'application/json'
    }

    response = requests.post(url, json={'query': mutation, 'variables': variables}, headers=headers)

    if response.status_code == 200:
        print(f"Anime with ID {anime_id} has been added to your watching list.")
    else:
        print(f"Failed to add anime. Status Code: {response.status_code}, Response: {response.text}")

def get_anime_mal_id(anilist_media_id):
    url = "https://graphql.anilist.co"
    query = '''
    query ($id: Int) {
      Media(id: $id) {
        idMal
      }
    }
    '''
    variables = {
        'id': anilist_media_id
    }

    # Make the API request
    response = requests.post(url, json={'query': query, 'variables': variables})
    
    if response.status_code == 200:
        data = response.json()
        malId = data['data']['Media']['idMal']
        return malId
    else:
        raise Exception(f"Failed to retrieve data from AniList API. Status Code: {response.status_code}")

def get_anime_id_and_image(anilist_media_id):
    url = "https://graphql.anilist.co"
    query = '''
    query ($id: Int) {
      Media(id: $id) {
        coverImage {
          large
        }
        idMal
      }
    }
    '''
    variables = {
        'id': anilist_media_id
    }

    # Make the API request
    response = requests.post(url, json={'query': query, 'variables': variables})
    
    if response.status_code == 200:
        data = response.json()
        image_url = data['data']['Media']['coverImage']['large']
        malId = data['data']['Media']['idMal']
        return malId, image_url
    else:
        raise Exception(f"Failed to retrieve data from AniList API. Status Code: {response.status_code}")

def get_user_data(access_token, user_id):
  query = """
  {
    MediaListCollection(userId: %s, type: ANIME) {
      lists {
        entries {
          media {
            id
            episodes
            duration
            title {
              romaji
              english
            }
          }
          status
          score
          progress
        }
      }
    }
  }
  """ % user_id

  # Send the request
  response = requests.post(
      'https://graphql.anilist.co',
      json={'query': query},
      headers={'Authorization': f'Bearer {access_token}'}
  )


  # Print the response
  return response.json()

def load_json_file(file_path):
    with open(file_path, 'r', encoding='utf-8') as file:
        return json.load(file)

def search_anime_by_title(json_data, search_title):
    results = []
    if search_title=="1P":
      search_title = "ONE PIECE"
    for list_item in json_data['data']['MediaListCollection']['lists']:
        for entry in list_item['entries']:
            media = entry['media']
            romaji_title = media['title']['romaji']
            english_title = media['title']['english']
            episodes = media['episodes']
            duration = media['duration']
            try:
              if search_title.lower() in romaji_title.lower() or search_title.lower() in english_title.lower():
                  results.append({
                      'id': media['id'],
                      'progress': entry['progress'],
                      'romaji_title': romaji_title,
                      'english_title': english_title,
                      'episodes': episodes,
                      'duration': duration,
                  })
            except:
              pass
    
    return results

def update_anime_progress(token: str, media_id: int, progress: int):
    url = "https://graphql.anilist.co"
    
    query = '''
    mutation($mediaId: Int, $progress: Int) {
        SaveMediaListEntry(mediaId: $mediaId, progress: $progress) {
            id
            progress
        }
    }
    '''
    
    variables = {
        "mediaId": media_id,  # The AniList ID of the anime
        "progress": progress  # The number of the latest episode you watched
    }
    
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "Accept": "application/json"
    }
    
    response = requests.post(url, json={"query": query, "variables": variables}, headers=headers)
    
    if response.status_code == 200:
        data = response.json()
        updated_progress = data['data']['SaveMediaListEntry']['progress']
        print("updating progress..")
        print(f"Anime progress updated! Latest watched episode: {updated_progress}")
    else:
        print(f"Error {response.status_code}: {response.text}")

def rate_anime(anilist_token, media_id, score):
    url = 'https://graphql.anilist.co'
    
    # GraphQL mutation to rate anime
    query = '''
    mutation($mediaId: Int, $score: Float) {
      SaveMediaListEntry(mediaId: $mediaId, score: $score) {
        id
        mediaId
        score
      }
    }
    '''
    
    # Variables for the mutation
    variables = {
        "mediaId": media_id,
        "score": float(score)
    }
    
    headers = {
        "Authorization": f"Bearer {anilist_token}",
        "Content-Type": "application/json",
    }
    
    response = requests.post(url, json={'query': query, 'variables': variables}, headers=headers)
    
    if response.status_code == 200:
        data = response.json()
        print(f"Successfully rated anime (mediaId: {media_id}) with score: {score}")
        return data
    else:
        print(f"Failed to rate anime. Status Code: {response.status_code}, Response: {response.text}")
        return None

# ----------------------------------------------- Select Anime Functions ----------------------------------------

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

def extract_anime_info(json_data):
    anime_info_list = []
    anime_info_dict = {}

    # Traverse the JSON structure to get to the list of entries
    for list_entry in json_data['data']['MediaListCollection']['lists']:
        for entry in list_entry['entries']:
            anime = entry['media']
            anime_info = {
                'id': anime['id'],
                'romaji_name': anime['title']['romaji'],
                'english_name': anime['title']['english'],
                'status': entry['status'],
                'episodes': anime['episodes'],
            }

            if anime['title']['english'] == None:
                anime['title']['english'] = anime['title']['romaji']
            # if anime['title'][''] == None:
            #     anime['title']['english'] = anime['title']['romanji']

            anime_info_dict[anime['title']['english']] = str(anime['id']) # {anime_name: anime_id, ...}
            anime_info_list.append(anime_info) # [{"id": anime_id, "romaji_name": anime_name_romnaji, "english_name": anime_name_english, status: anime_status}, ...]

    return [anime_info_dict, anime_info_list]

def select_anime(anime_list):
    def main(stdscr):
        # Clear screen
        curses.curs_set(1)
        stdscr.clear()
        stdscr.refresh()

        # Initial values
        current_input = ""
        filtered_list = list(anime_list.keys())
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
                        selected_id = anime_list[selected_anime]
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
                filtered_list = [anime for anime in anime_list.keys() if current_input.lower() in anime.lower()]

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
        # print(f"Selected Anime: {selected_anime}, ID: {selected_id}")
        with open(f"/tmp/curd/curd_anime", "w") as anime_name:
            anime_name.write(selected_anime)

        with open(f"/tmp/curd/curd_id", "w") as id_file:
            id_file.write(str(selected_id))
        
        return True

    # Initialize curses
    try:
        curses.wrapper(main)
    except:
        pass

# ----------------------------------------------- Select Link Functions ----------------------------------------


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

# ----------------------------------------------- Start Video Functions ----------------------------------------

def start_video(link, salt:str, args:list=[]):
    # print(f"SALT IS {salt}")

    args_str = ' '.join(args)
    
    # Build the complete command string
    command = f"mpv {args_str} --no-terminal --really-quiet --input-ipc-server=/tmp/curd/curd_mpvsocket{salt} {link}"

    subprocess.Popen(command, shell=True)

def mpv_send_command(ipc_socket_path, command):
    """
    Sends a command to the MPV IPC socket and returns the response.
    """
    with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as s:
        s.connect(ipc_socket_path)
        command_str = json.dumps({"command": command}) + "\n"
        s.sendall(command_str.encode())
        response = s.recv(4096).decode()

        try:
            response_data = json.loads(response)
            if 'data' in response_data:
                return response_data['data']
        except json.JSONDecodeError:
            return None
    return None

def seek_mpv(ipc_socket_path, time):
    """Seek MPV to a specific time."""
    command = ["seek", time, "absolute"]
    return mpv_send_command(ipc_socket_path, command)


def get_mpv_paused_status(ipc_socket_path):
    status = mpv_send_command(ipc_socket_path, ["get_property", "pause"])
    if status is not None:
        return status
    else:
        return False

def get_mpv_playback_speed(ipc_socket_path):
    current_speed = mpv_send_command(ipc_socket_path, ["get_property", "speed"])
    if current_speed is not None:
        return current_speed
    else:
        print("Failed to get playback speed.")

def get_percentage_watched(ipc_socket_path):
    """
    Calculates the percentage watched of the currently playing video.
    """
    # Get current playback time and total duration
    current_time = mpv_send_command(ipc_socket_path, ["get_property", "time-pos"])
    duration = mpv_send_command(ipc_socket_path, ["get_property", "duration"])

    if current_time is not None and duration is not None and duration > 0:
        percentage_watched = (current_time / duration) * 100
        return percentage_watched
    return 0

def percentage_watched(playback_time:int, duration:int):
    if playback_time is not None and duration is not None and duration > 0:
        video_percentage_watched = (playback_time/duration) * 100
        return video_percentage_watched
    return None

# ----------------------------------------------- Track Anime Functions ----------------------------------------
def add_anime(database_file, anilist_id, allanime_id, episode, time, duration, name):
    with open(database_file, mode='a', newline='') as file:
        writer = csv.writer(file)
        writer.writerow([anilist_id, allanime_id, episode, time, duration, name])
    print("Written to file")

# Function to delete an anime entry by Anilist ID and Allanime ID
def delete_anime(database_file, anilist_id, allanime_id):
    anime_list = []
    with open(database_file, mode='r', newline='') as file:
        reader = csv.reader(file)
        for row in reader:
            # Keep only entries that don't match the Anilist ID and Allanime ID
            if row[0] != anilist_id or row[1] != allanime_id:
                anime_list.append(row)

    # Rewrite the database without the deleted entry
    with open(database_file, mode='w', newline='') as file:
        writer = csv.writer(file)
        writer.writerows(anime_list)

# Function to retrieve all anime entries
def get_all_anime(database_file):
    anime_list = []
    with open(database_file, mode='r', newline='') as file:
        reader = csv.reader(file)
        for row in reader:
            anime_list.append({
                'anilist_id': row[0],
                'allanime_id': row[1],
                'episode': row[2],
                'time': row[3],
                'duration': row[4],
                'name': row[5]
            })
    return anime_list

# Function to update or add a new anime entry
def update_anime(database_file: str, anilist_id:str, allanime_id: str, episode:str, time:str, duration:str, name:str):
    anime_list = get_all_anime(database_file)
    updated = False

    # Create a new list to store updated entries
    updated_anime_list = []
    
    # Check if the anime entry exists and update it
    for anime in anime_list:
        if anime['anilist_id'] == anilist_id and anime['allanime_id'] == allanime_id:
            updated_anime_list.append({
                'anilist_id': anilist_id,
                'allanime_id': allanime_id,
                'episode': episode,
                'time': time,
                'duration': duration,
                'name': name
            })
            updated = True
        else:
            updated_anime_list.append(anime)
    
    # If no entry was updated, add new entry
    if not updated:
        updated_anime_list.append({
            'anilist_id': anilist_id,
            'allanime_id': allanime_id,
            'episode': episode,
            'time': time,
            'duration': duration,
            'name': name
        })

    # Write all entries back to the file
    with open(database_file, mode='w', newline='') as file:
        writer = csv.writer(file)
        for anime in updated_anime_list:
            writer.writerow([anime['anilist_id'], anime['allanime_id'], anime['episode'], anime['time'], anime['duration'], anime['name']])
    
    # print("Updated the file")

def find_anime(anime_list, anilist_id=-1, allanime_id=-1):
    for anime in anime_list:
        if anime['anilist_id'] == anilist_id or anime['allanime_id'] == allanime_id:
            return anime
        
    return False

# ----------------------------------------------- Curd Functions ----------------------------------------

def print_error(message):
    """Print error message with full traceback."""
    print(f"ERROR: {message}")
    print("Traceback:")
    traceback.print_exc()

def get_contents_of(tmp_file_name):
    with open(f"/tmp/curd/curd_{tmp_file_name}", "r") as temp_file:
        return temp_file.read()

def run_script(script: str):
    os.system(f"/tmp/curd/{script}.sh")

def write_to_tmp(tmp_filename:str, content:str):
    try:
        with open(f"/tmp/curd/curd_{tmp_filename}", "w") as _:
            _.write(content)
        return True
    except FileNotFoundError:
        with open(f"/tmp/curd/curd_{tmp_filename}", "w") as _:
            _.write(content)
        return True
    except:
        print("fuck")
        return False

def read_tmp(tmp_filename:str):
    try:
        with open(f"/tmp/curd/curd_{tmp_filename}", "r") as _:
            content = _.read()
            return content
    except FileNotFoundError:
        with open(f"/tmp/curd/curd_{tmp_filename}", "w") as _:
            _.write("")
            return ""
    except:
        print("fuck")
        return False

def download_anilist_data(access_token, user_id):
    ''' dowlnoad anilist user data'''
    # print("downloading user data")
    anilist_user_data = get_user_data(access_token, user_id)
    try:
        if anilist_user_data['data'] == None:
            print("Cannot process user data.")
            # print(anilist_user_data)
            exit()
    except:
        pass

    return anilist_user_data

default_config = {
    "player":"mpv",
    "show_adult_content":False,
    "history_file":"$HOME/.local/share/curd/curd_history.txt",
    "subs_language":"english",
    "sub_or_dub":"sub",
    "percentage_to_mark_complete":85,
    "next_episode_prompt": False,
    "score_on_completion": True,
    "save_mpv_speed": True,
    "discord_presence": True,
    "presence_script_path":"curddiscordpresence.py"
}

def convert_seconds_to_minutes(seconds) -> str:
    minutes = seconds // 60
    remaining_seconds = seconds % 60
    if remaining_seconds < 10:
        return f"{minutes}:0{remaining_seconds}"
    return f"{minutes}:{remaining_seconds}"

def get_userconfig_value(userconfig:dict, key:str):
    return userconfig.get(key, default_config.get(key))

def create_default_user_config(default_config):
    output_string = ""
    for key, value in default_config.items():
        # Convert Python True/False to lowercase true/false
        if isinstance(value, bool):
            value_str = 'true' if value else 'false'
        else:
            value_str = str(value)
            
        # Write the key=value to the file with no spaces
        output_string+=f"{key}={value_str}\n"

    return output_string

def load_config() -> dict:
    command = """echo $HOME"""

    result = subprocess.run(command, shell=True, capture_output=True, text=True)
    output = result.stdout.strip()

    folder_name = f"{output}/.config/curd"
    file_name = "curd.conf"

    config_file_path = os.path.join(folder_name, file_name)

    # print(config_file_path)
    if not os.path.exists(os.path.join(config_file_path)):
        print("Creating config")
        
        try:
            print("making folder")
            os.makedirs(folder_name)
        except:
            print("error making folder")
            pass

        with open(config_file_path, 'w') as file:
            file.write(create_default_user_config(default_config))
            
    config_dict = {}

    with open(config_file_path, 'r') as file:
        for line in file:
            # Strip whitespace and ignore comments or empty lines
            line = line.strip()
            if line and not line.startswith("#"):
                # Split the line into key and value
                key, value = line.split("=", 1)
                key = key.strip()
                value = value.strip().strip('"')
                
                # Handle environment variables in the value
                value = os.path.expandvars(value)
                
                # Convert boolean strings to actual booleans
                if value.lower() == "true":
                    value = True
                elif value.lower() == "false":
                    value = False
                
                try:
                    value = int(value)
                except Exception as e:
                    # print_error("878: {e}")
                    pass

                # Store in the dictionary
                config_dict[key] = value

    if not os.path.exists(config_dict['history_file']):
        try:
            os.makedirs(os.path.dirname(config_dict['history_file']))
        except Exception as e:
            print_error("888: {e}")
        print("creating history file")
        with open(config_dict["history_file"], "w") as history_file:
            history_file.write("")

    return config_dict

# ----------------------------------------------- Create Bash Script ----------------------------------------

def create_anime_list():
    script_path = "/tmp/curd/anime_list.sh"
    with open(script_path, "w") as _:
        _.write(r"""search_anime() {
    search_gql="query(        \$search: SearchInput        \$limit: Int        \$page: Int        \$translationType: VaildTranslationTypeEnumType        \$countryOrigin: VaildCountryOriginEnumType    ) {    shows(        search: \$search        limit: \$limit        page: \$page        translationType: \$translationType        countryOrigin: \$countryOrigin    ) {        edges {            _id name availableEpisodes __typename       }    }}"

    curl -e "$allanime_refr" -s -G "${allanime_api}/api" --data-urlencode "variables={\"search\":{\"allowAdult\":false,\"allowUnknown\":false,\"query\":\"$1\"},\"limit\":40,\"page\":1,\"translationType\":\"$mode\",\"countryOrigin\":\"ALL\"}" --data-urlencode "query=$search_gql" -A "$agent" | sed 's|Show|\
|g' | sed -nE "s|.*_id\":\"([^\"]*)\",\"name\":\"([^\"]*)\".*${mode}\":([1-9][^,]*).*|\1	\2 (\3 episodes)NEWLINEFROMHERE|p"
}

agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"
allanime_refr="https://allanime.to"
allanime_base="allanime.day"
allanime_api="https://api.${allanime_base}"
mode="${ANI_CLI_MODE:-sub}"
download_dir="${ANI_CLI_DOWNLOAD_DIR:-.}"
quality="${ANI_CLI_QUALITY:-best}"
case "$(uname -a)" in
    *Darwin*) player_function="${ANI_CLI_PLAYER:-iina}" ;;            # mac OS
    *ndroid*) player_function="${ANI_CLI_PLAYER:-android_mpv}" ;;     # Android OS (termux)
    *steamdeck*) player_function="${ANI_CLI_PLAYER:-flatpak_mpv}" ;;  # steamdeck OS
    *MINGW* | *WSL2*) player_function="${ANI_CLI_PLAYER:-mpv.exe}" ;; # Windows OS
    *ish*) player_function="${ANI_CLI_PLAYER:-iSH}" ;;                # iOS (iSH)
    *) player_function="${ANI_CLI_PLAYER:-mpv}" ;;                    # Linux OS
esac

# Read the entire file content into a variable
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
query=$(cat /tmp/curd/curd_query)  # This is a shorthand for cat "$filename"

# query="one piece"
anime_list=$(search_anime "$query")

echo $anime_list > /tmp/curd/curd_anime_list
""")
    os.chmod(script_path, 0o755)

def create_episode_list():
    script_path = "/tmp/curd/episode_list.sh"
    with open(script_path, "w") as _:
        _.write(r"""episodes_list() {
    episodes_list_gql="query (\$showId: String!) {    show(        _id: \$showId    ) {        _id availableEpisodesDetail    }}"

    curl -e "$allanime_refr" -s -G "${allanime_api}/api" --data-urlencode "variables={\"showId\":\"$*\"}" --data-urlencode "query=$episodes_list_gql" -A "$agent" | sed -nE "s|.*$mode\":\[([0-9.\",]*)\].*|\1|p" | sed 's|,|\
|g; s|"||g' | sort -n -k 1
}

# setup
agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"
allanime_refr="https://allanime.to"
allanime_base="allanime.day"
allanime_api="https://api.${allanime_base}"
mode="${ANI_CLI_MODE:-sub}"
download_dir="${ANI_CLI_DOWNLOAD_DIR:-.}"
quality="${ANI_CLI_QUALITY:-best}"
case "$(uname -a)" in
    *Darwin*) player_function="${ANI_CLI_PLAYER:-iina}" ;;            # mac OS
    *ndroid*) player_function="${ANI_CLI_PLAYER:-android_mpv}" ;;     # Android OS (termux)
    *steamdeck*) player_function="${ANI_CLI_PLAYER:-flatpak_mpv}" ;;  # steamdeck OS
    *MINGW* | *WSL2*) player_function="${ANI_CLI_PLAYER:-mpv.exe}" ;; # Windows OS
    *ish*) player_function="${ANI_CLI_PLAYER:-iSH}" ;;                # iOS (iSH)
    *) player_function="${ANI_CLI_PLAYER:-mpv}" ;;                    # Linux OS
esac
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
id=$(cat /tmp/curd/curd_id)
echo $(episodes_list "$id") > /tmp/curd/curd_episode_list
""")
    os.chmod(script_path, 0o755)

def create_episode_url():
    script_path = "/tmp/curd/episode_url.sh"
    with open(script_path, "w") as _:
        _.write(r"""# # extract the video links from response of embed urls, extract mp4 links form m3u8 lists
get_links() {
    episode_link="$(curl -e "$allanime_refr" -s "https://${allanime_base}$*" -A "$agent" | sed 's|},{|\
|g' | sed -nE 's|.*link":"([^"]*)".*"resolutionStr":"([^"]*)".*|\2 >\1|p;s|.*hls","url":"([^"]*)".*"hardsub_lang":"en-US".*|\1|p')"

    case "$episode_link" in
        *repackager.wixmp.com*)
            extract_link=$(printf "%s" "$episode_link" | cut -d'>' -f2 | sed 's|repackager.wixmp.com/||g;s|\.urlset.*||g')
            for j in $(printf "%s" "$episode_link" | sed -nE 's|.*/,([^/]*),/mp4.*|\1|p' | sed 's|,|\
|g'); do
                printf "%s >%s\n" "$j" "$extract_link" | sed "s|,[^/]*|${j}|g"
            done | sort -nr
            ;;
        *vipanicdn* | *anifastcdn*)
            if printf "%s" "$episode_link" | head -1 | grep -q "original.m3u"; then
                printf "%s" "$episode_link"
            else
                extract_link=$(printf "%s" "$episode_link" | head -1 | cut -d'>' -f2)
                relative_link=$(printf "%s" "$extract_link" | sed 's|[^/]*$||')
                curl -e "$allanime_refr" -s "$extract_link" -A "$agent" | sed 's|^#.*x||g; s|,.*|p|g; /^#/d; $!N; s|\
| >|' | sed "s|>|>${relative_link}|g" | sort -nr
            fi
            ;;
        *) [ -n "$episode_link" ] && printf "%s\n" "$episode_link" ;;
    esac
    # echo $episode_link >> links
    # [ -z "$ANI_CLI_NON_INTERACTIVE" ] && printf "\033[1;32m%s\033[0m Links Fetched\n" "$provider_name" 1>&2
    [ -z "$ANI_CLI_NON_INTERACTIVE" ] 1>&2
}

# # innitialises provider_name and provider_id. First argument is the provider name, 2nd is the regex that matches that provider's link
provider_init() {
    provider_name=$1
    provider_id=$(printf "%s" "$resp" | sed -n "$2" | head -1 | cut -d':' -f2 | sed 's/../&\
/g' | sed 's/^01$/9/g;s/^08$/0/g;s/^05$/=/g;s/^0a$/2/g;s/^0b$/3/g;s/^0c$/4/g;s/^07$/?/g;s/^00$/8/g;s/^5c$/d/g;s/^0f$/7/g;s/^5e$/f/g;s/^17$/\//g;s/^54$/l/g;s/^09$/1/g;s/^48$/p/g;s/^4f$/w/g;s/^0e$/6/g;s/^5b$/c/g;s/^5d$/e/g;s/^0d$/5/g;s/^53$/k/g;s/^1e$/\&/g;s/^5a$/b/g;s/^59$/a/g;s/^4a$/r/g;s/^4c$/t/g;s/^4e$/v/g;s/^57$/o/g;s/^51$/i/g;' | tr -d '\n' | sed "s/\/clock/\/clock\.json/")
}

# # +s links based on given provider
generate_link() {
    case $1 in
        1) provider_init "wixmp" "/Default :/p" ;;     # wixmp(default)(m3u8)(multi) -> (mp4)(multi)
        2) provider_init "dropbox" "/Sak :/p" ;;       # dropbox(mp4)(single)
        3) provider_init "wetransfer" "/Kir :/p" ;;    # wetransfer(mp4)(single)
        4) provider_init "sharepoint" "/S-mp4 :/p" ;;  # sharepoint(mp4)(single)
        *) provider_init "gogoanime" "/Luf-mp4 :/p" ;; # gogoanime(m3u8)(multi)
    esac
    [ -n "$provider_id" ] && get_links "$provider_id"
    # echo $links >> links
}

select_quality() {
    case "$1" in
        best) result=$(printf "%s" "$links" | head -n1) ;;
        worst) result=$(printf "%s" "$links" | grep -E '^[0-9]{3,4}' | tail -n1) ;;
        *) result=$(printf "%s" "$links" | grep -m 1 "$1") ;;
    esac
    [ -z "$result" ] && printf "Specified quality not found, defaulting to best\n" 1>&2 && result=$(printf "%s" "$links" | head -n1)
    printf "%s" "$result" | cut -d'>' -f2
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# # gets embed urls, collects direct links into provider files, selects one with desired quality into $episode
get_episode_url() {
    # get the embed urls of the selected episode
    episode_embed_gql="query (\$showId: String!, \$translationType: VaildTranslationTypeEnumType!, \$episodeString: String!) {    episode(        showId: \$showId        translationType: \$translationType        episodeString: \$episodeString    ) {        episodeString sourceUrls    }}"

    resp=$(curl -e "$allanime_refr" -s -G "${allanime_api}/api" --data-urlencode "variables={\"showId\":\"$id\",\"translationType\":\"$mode\",\"episodeString\":\"$ep_no\"}" --data-urlencode "query=$episode_embed_gql" -A "$agent" | tr '{}' '\n' | sed 's|\\u002F|\/|g;s|\\||g' | sed -nE 's|.*sourceUrl":"--([^"]*)".*sourceName":"([^"]*)".*|\2 :\1|p')
    # generate links into sequential files
    cache_dir="$(mktemp -d)"
    providers="1 2 3 4 5"
    for provider in $providers; do
        generate_link "$provider" >"$cache_dir"/"$provider" &
    done
    wait
    # echo $links > links
    # select the link with matching quality
    links=$(cat "$cache_dir"/* | sed 's|^Mp4-||g;/http/!d' | sort -g -r -s)
    rm -r "$cache_dir"
    episode=$(select_quality "$quality")
    echo $links > /tmp/curd/curd_links
    [ -z "$episode" ] && die "Episode not released!"
}

# setup
agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"
allanime_refr="https://allanime.to"
allanime_base="allanime.day"
allanime_api="https://api.${allanime_base}"
sub_or_dub=$(cat /tmp/curd/curd_mode)
mode="${ANI_CLI_MODE:-$sub_or_dub}"
download_dir="${ANI_CLI_DOWNLOAD_DIR:-.}"
quality="${ANI_CLI_QUALITY:-best}"
case "$(uname -a)" in
    *Darwin*) player_function="${ANI_CLI_PLAYER:-iina}" ;;            # mac OS
    *ndroid*) player_function="${ANI_CLI_PLAYER:-android_mpv}" ;;     # Android OS (termux)
    *steamdeck*) player_function="${ANI_CLI_PLAYER:-flatpak_mpv}" ;;  # steamdeck OS
    *MINGW* | *WSL2*) player_function="${ANI_CLI_PLAYER:-mpv.exe}" ;; # Windows OS
    *ish*) player_function="${ANI_CLI_PLAYER:-iSH}" ;;                # iOS (iSH)
    *) player_function="${ANI_CLI_PLAYER:-mpv}" ;;                    # Linux OS
esac
id=$(cat /tmp/curd/curd_id)
ep_no=$(cat /tmp/curd/curd_ep_no)
get_episode_url 
""")
    os.chmod(script_path, 0o755)

# ----------------------------------------------- Start of script ----------------------------------------

create_anime_list()
create_episode_list()
create_episode_url()

current_dir = Path(__file__).parent
# current_dir = os.path.dirname(os.path.abspath(__file__))
# print(current_dir)
if not os.path.exists(f"/tmp/curd/"):
    try:
        os.makedirs(os.path.dirname(f"/tmp/curd/"))
    except:
        pass

rewatching = False

parser = argparse.ArgumentParser(description="Print a greeting message.")
# parser.add_argument("--name", type=str, required=True, help="Your name")
parser.add_argument("-new", action='store_true', help="Add new anime to watching (optional)")
parser.add_argument("-sub", action='store_true', help="Anime audio type (optional)")
parser.add_argument("-dub", action='store_true', help="Anime audio type (optional)")
args = parser.parse_args()

user_config = load_config() # python dictionary containing all the configs as key value pairs
history_file_path_ = os.path.expandvars(get_userconfig_value(user_config, "history_file"))
history_file_path_ = os.path.dirname(history_file_path_)
access_token_path = os.path.join(history_file_path_, "token")

if os.path.exists(access_token_path):
    # Read and use the token
    with open(access_token_path, "r") as token_file:
        access_token = token_file.read().strip()
    # print(f"Token found: {access_token}")

else:
    print("Generate the token from https://anilist.co/api/v2/oauth/authorize?client_id=20686&response_type=token ")
    access_token = input("Token file not found. Please generate and enter your token: ")
    with open(access_token_path, "w") as token_file:
        token_file.write(access_token)
    print(f"Token saved to {access_token_path}")

if access_token == None:
    print("No Access_token provided.")
    exit(1)

if args.new:
    new_anime_name_ = input("Enter anime name: ")
    temp__ = search_anime_anilist(new_anime_name_, access_token)
    if temp__ :
        select_anime(temp__)
        add_anime_to_watching_list(read_tmp('id'), access_token)
    else:
        print("No anime found")

user_id, user_name = get_anilist_user_id(access_token)
mark_episode_as_completed_at = get_userconfig_value(user_config, "percentage_to_mark_complete")

anilist_user_data = download_anilist_data(access_token, user_id)
anime_dict = extract_anime_info(anilist_user_data)[0]
select_anime(anime_dict)
anime_name = read_tmp("anime")
write_to_tmp("query", anime_name)
run_script("anime_list")
anime_dict = load_anime_data(f"/tmp/curd/curd_anime_list")
cleaned_text = re.sub(r'\(.*$', '', anime_name).strip() # clean anime name
# print(cleaned_text)
try:
    result = search_anime_by_title(anilist_user_data, cleaned_text)[0]
    # print(result)
    media_id = result['id']
    progress = int(result['progress'])
    title = result['english_title']
    total_episodes = result['episodes']
    duration = result['duration']
except KeyboardInterrupt:
    print("bye")
    exit(0)
except Exception as e:
    print(f"Searching anime error: {e}")
    exit(1)

try:
    finding_anime = find_anime(get_all_anime(get_userconfig_value(user_config, "history_file")), anilist_id=str(media_id))
    # print(finding_anime)
    if finding_anime:
        print("Found anime in local history")
        write_to_tmp("id", finding_anime['allanime_id'])
        write_to_tmp("anime", finding_anime['name'])
    else:
        print("Trying to automate")
        write_to_tmp("id", anime_dict[anime_name+f" ({total_episodes} episodes)"])
        write_to_tmp("anime", anime_name)
except KeyboardInterrupt:
    print("bye")
    exit(0)
except Exception as e:
    # print_error(e)
    print("Please select anime")
    select_anime(anime_dict)

if get_userconfig_value(user_config, "sub_or_dub")=="sub":
    write_to_tmp("mode", "sub")
elif get_userconfig_value(user_config, "sub_or_dub")=="dub":
    write_to_tmp("mode", "dub")

if args.sub: # arguments take precidence
    write_to_tmp("mode", "sub")
if args.dub:
    write_to_tmp("mode", "dub")

run_script("episode_list")

binge_watching = False
mpv_args = []
last_episode = int(read_tmp("episode_list").split()[-1])
anime_watch_history = get_all_anime(get_userconfig_value(user_config, 'history_file'))
anime_history = find_anime(anime_watch_history, anilist_id=media_id, allanime_id=get_contents_of("id"))
episode_completed = False

if anime_history: # if it exists in local history
    # print(f"came in history {str(progress)}")
    # print(anime_history['episode'])
    if progress == last_episode or int(anime_history['episode']) == int(progress)+1:
        if progress == last_episode:
            rewatching = True
        mpv_args.append(f"--start={anime_history['time']}")
        watching_ep = int(anime_history['episode'])
        write_to_tmp("ep_no", str(watching_ep))
        # print(f"STARTING ANIME FROM EPISODE {anime_history['episode']} {anime_history['time']}")
    elif int(anime_history['episode']) < progress+1: # if the upstream progress is ahead
        write_to_tmp("ep_no", str(int(progress)+1))
        print(f"Starting anime from upstream {str(progress + 1)}")
        watching_ep = progress + 1
else: # if history does not exist
    if progress == last_episode:
        user_input = input("Do you want to start this anime from beginning? (y/n):")
        if user_input.lower() == "yes" or user_input.lower() == "y" or user_input.lower() == "":
            progress = 0
            rewatching = True
        else:
            print("Starting last episode of anime")
            progress = last_episode - 1
    watching_ep = int(progress)+1
    write_to_tmp("ep_no", str(watching_ep))
# print(watching_ep)
# os.system("{current_dir}/scripts/episode_url.sh")
run_script("episode_url")

# Print the result
if media_id:
    # print(f"Anime ID: {media_id}")
    pass
else:
    print("Anime not found.")
    exit(1)

links = load_links(f"/tmp/curd/curd_links")

if get_userconfig_value(user_config, "discord_presence") == True:
    from pypresence import Presence
    # print("imported pypresence")
    rpc = Presence(discord_client_id)
    rpc.connect()
    anime_image_url = get_anime_image(media_id)

while True:

    try:
        salt = random.randint(0,1500)
        # print("SALT IS:"+str(salt))
        start_video(links[0][1], salt, mpv_args)
        mpv_socket_path = "/tmp/curd/curd_mpvsocket"+str(salt)
        connect_mpv_command = """echo '{ "command": ["get_property", "playback-time"] }' | socat - """+mpv_socket_path
        is_paused = False
        while True:
            time.sleep(1)
            result = subprocess.run(connect_mpv_command, shell=True, capture_output=True, text=True)
            # print(result)
            if result.returncode == 0:
                output = result.stdout.strip()
                if not output:  # Check if output is empty
                    print("No data received. Retrying...")
                try:
                    time.sleep(1)
                    data = json.loads(output)
                    if data["error"] == "success":
                        playback_time = round(int(data["data"]), 2)
                        if get_userconfig_value(user_config, "discord_presence") == True:
                            mpv_status = get_mpv_paused_status(mpv_socket_path)
                            if mpv_status == True:
                                is_paused = True
                            else:
                                is_paused = False

                            # print("mpv_status", mpv_status)
                            # print("is_paused", is_paused)
                            if is_paused:
                                # Update presence with paused status
                                rpc.update(
                                    details=f"Watching {title}",
                                    state=f"Episode {watching_ep} - {convert_seconds_to_minutes(playback_time)} (Paused)",
                                    large_image=anime_image_url,
                                    large_text=title,
                                )
                            else:
                                # Update presence with playback time
                                rpc.update(
                                    details=f"Watching {title}",
                                    state=f"Episode {watching_ep} - {convert_seconds_to_minutes(playback_time)}/{duration}:00",
                                    large_image=anime_image_url,
                                    large_text=title,
                                )

                        update_anime(get_userconfig_value(user_config, 'history_file'), str(media_id), str(get_contents_of("id")), str(watching_ep), str(playback_time), str(duration), str(title))
                        # print("Playback time:", playback_time)
                        watched_percentage = get_percentage_watched(mpv_socket_path)
                        mpv_playback_speed = get_mpv_playback_speed(mpv_socket_path)

                        if watched_percentage > mark_episode_as_completed_at:
                            episode_completed = True
                            binge_watching = True
                        # else:
                            # print(f"Episode not done: {watched_percentage} {mark_episode_as_completed_at}")
                        # video_duration = subprocess.run(mpv_duration_command, shell=True, capture_output=True, text=True):
                    elif data['error'] == "property unavailable": # Stream has not started yet
                        pass
                    else:
                        print("Error (330):", data["error"])
                        break
                except json.decoder.JSONDecodeError as e:
                    print("Error decoding JSON:", e)
                    break  # Exit the loop on unexpected JSON error
                except KeyboardInterrupt:
                    print("bye")
                    exit(0)
                except Exception as e:
                    print_error(f"Unknown:\n {e}")
                    print("Unknown:\n", e)
            else:
                killing_error = str(result.stderr)
                # print(killing_error[-19:-1])
                try:
                    if killing_error[-19:-1] == "Connection refused": # user closed the stream
                        if watched_percentage > mark_episode_as_completed_at:
                            # update_anime_progress(access_token, int(media_id), int(watching_ep))
                            # watching_ep = int(watching_ep)+1
                            # update_anime(user_config['history_file'], str(media_id), str(get_contents_of("id")), str(watching_ep), "0", str(duration), str(title))
                            break
                        else:
                            print("Have a great day!")
                            exit(0)

                except Exception as e: # user did not close the stream
                    print(e)
                if killing_error == "property unavailable": # mpv is not started yet
                    # print("passing")
                    pass
                else: # Stream has ended (maybe)
                    print("Error (346):", killing_error)
                    # try:

                    # except Exception as e:
                    #     print("fuck")
                    #     print(f"Exception: {e}")
                    break
    except ConnectionRefusedError: # execption is never reached ig
        print("Player Closed")
        # print("have a great")
        # exit(0)
    except KeyboardInterrupt:
        if watched_percentage > mark_episode_as_completed_at and not rewatching:
            update_anime_progress(access_token, int(media_id), int(watching_ep))
            watching_ep = int(watching_ep)+1
            update_anime(get_userconfig_value(user_config, 'history_file'), str(media_id), str(get_contents_of("id")), str(watching_ep), "0", str(duration), str(title))

        print("bye")
        exit(0)
    finally:
        binge_watching=True
    try:
        # print("binge watching")
        current_ep = int(read_tmp("ep_no"))

        if current_ep == last_episode:
            if binge_watching == True and not rewatching and get_userconfig_value(user_config, 'score_on_completion') == True: # Completed anime
                anime_rating_by_user = input("Rate this anime:\n")
                update_anime_progress(access_token, int(media_id), int(watching_ep))
                rate_anime(access_token, media_id, anime_rating_by_user)
            print("completed anime.")
            delete_anime(get_userconfig_value(user_config, 'history_file'), media_id, get_contents_of("id"))
            exit(0)

        else:
            print(f"Starting next episode: {watching_ep}")
            # write_to_tmp("ep_no", str(int(current_ep)+1))
        
        if watched_percentage > mark_episode_as_completed_at: # IF BINGE WATCHING
            last_episode = int(read_tmp("episode_list").split()[-1])
            if not rewatching:
                update_anime_progress(access_token, int(media_id), int(watching_ep))
            watching_ep = int(watching_ep)+1
            write_to_tmp("ep_no", str(watching_ep))
            update_anime(get_userconfig_value(user_config, 'history_file'), str(media_id), str(get_contents_of("id")), str(watching_ep), "0", str(duration), str(title))
            anime_watch_history = get_all_anime(get_userconfig_value(user_config, 'history_file'))
            anime_history = find_anime(anime_watch_history, anilist_id=media_id, allanime_id=get_contents_of("id"))
            episode_completed = False
            print("Fetching anime")
            run_script("episode_url")
            links = load_links(f"/tmp/curd/curd_links")
            mpv_args = []
            watched_percentage = 0

            if get_userconfig_value(user_config, 'save_mpv_speed') ==True:
                mpv_args.append(f'--speed={mpv_playback_speed}')

        # watching_ep = current_ep+1
            if get_userconfig_value(user_config, 'next_episode_prompt') == True:
                to_continue_or_not_to_continue = input("Continue? (y/n)\n")
                if to_continue_or_not_to_continue.lower() == "yes" or to_continue_or_not_to_continue.lower() == "y" or to_continue_or_not_to_continue == "":
                    binge_watching = True
                else:
                    exit(0)
    except Exception as e:
        # print(f"error:{e}\nMaybe try running again!")
        print_error(f"{e}")
        print("Maybe try running again!")
        exit(1)