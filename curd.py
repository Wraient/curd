# if you are trying to read this code, god help you.
# this is how my implementation of the ani-cli code to get the anime works

# search (anime_list.sh) -> tmp/anime_list 
# anime_id -> episode_list.sh
# episode_number -> episdoe_url.sh -> tmp/links 

import re
import os
import json
import random
import subprocess
import time

from anilist import search_anime_by_title
from anilist import get_user_data, get_anilist_user_id, update_anime_progress, rate_anime
from select_link import load_links
from start_video import start_video, send_command, get_percentage_watched, percentage_watched, get_mpv_playback_speed
from select_anime import load_anime_data
from select_anime import extract_anime_info
from select_anime import select_anime

from track_anime import add_anime, update_anime, get_all_anime, delete_anime, find_anime

mark_episode_as_completed_at = 85

access_token = os.environ.get('ANILIST_ACCESS_TOKEN')
user_id, user_name = get_anilist_user_id(access_token)
# user_id = os.environ.get('ANILIST_USER_ID')

if access_token == None:
    print("No Access_token provided.")
    exit(1)

# print(access_token)
# print(user_id)

def get_contents_of(tmp_file_name):
    with open(f"scripts/tmp/{tmp_file_name}", "r") as temp_file:
        return temp_file.read()

def run_script(script: str):
    os.system(f"./scripts/{script}.sh")

def write_to_tmp(tmp_filename:str, content:str):
    try:
        with open(f"scripts/tmp/{tmp_filename}", "w") as _:
            _.write(content)
        return True
    except:
        return False

def read_tmp(tmp_filename:str):
    try:
        with open(f"scripts/tmp/{tmp_filename}", "r") as _:
            content = _.read()
            return content
    except:
        return False
    
def download_anilist_data(access_token, user_id):
    ''' dowlnoad anilist user data'''
    # print("downloading user data")
    anilist_user_data = get_user_data(access_token, user_id)
    with open("response.json", "w") as response:
        response.write(str(anilist_user_data))
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
    "next_episode_prompt":False,
    "score_on_completion":False,
    "save_mpv_speed": True,
    "discord_presence": True,
    "presence_script_path":"curddiscordpresence.py"
}

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
    command = """echo $(xdg-user-dir CONFIG)"""

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
                
                # Store in the dictionary
                config_dict[key] = value

    if not os.path.exists(config_dict['history_file']):
        try:
            os.makedirs(os.path.dirname(config_dict['history_file']))
        except:
            pass
        print("creating history file")
        with open(config_dict["history_file"], "w") as history_file:
            history_file.write("")

    return config_dict

    # else:
    # with open(config_file_path, "r") as config_file:
    #     return config_file.read()

# START OF SCRIPT

user_config = load_config() # python dictionary containing all the configs as key value pairs
anilist_user_data = download_anilist_data(access_token, user_id)
anime_dict = extract_anime_info(anilist_user_data)[0]
select_anime(anime_dict)
anime_name = read_tmp("anime")
write_to_tmp("query", anime_name)
run_script("anime_list")
anime_dict = load_anime_data("scripts/tmp/anime_list")
cleaned_text = re.sub(r'\(.*$', '', anime_name).strip() # clean anime name

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
        print("Found anime in history")
        write_to_tmp("id", finding_anime['allanime_id'])
        write_to_tmp("anime", finding_anime['name'])
    else:
        print("Trying to automate")
        write_to_tmp("id", anime_dict[anime_name+f" ({total_episodes} episodes)"])
        write_to_tmp("anime", anime_name)
except KeyboardInterrupt:
    print("bye")
    exit(0)
except:
    print("Please select anime")
    select_anime(anime_dict)

run_script("episode_list")

binge_watching = False
mpv_args = []
last_episode = int(read_tmp("episode_list").split()[-1])
anime_watch_history = get_all_anime(get_userconfig_value(user_config, 'history_file'))
anime_history = find_anime(anime_watch_history, anilist_id=media_id, allanime_id=get_contents_of("id"))
episode_completed = False
rewatching = False

if anime_history: # if it exists in local history
    # print(f"came in history {str(progress)}")
    if int(anime_history['episode']) != progress+1: # if the upstream progress is ahead
        write_to_tmp("ep_no", str(int(progress)+1))
        # print(f"Starting anime from upstream {str(progress + 1)}")
        watching_ep = progress + 1

    elif int(anime_history['episode']) == int(progress)+1: # if the upstream progress is NOT ahead
        mpv_args.append(f"--start={anime_history['time']}")
        write_to_tmp("ep_no", anime_history['episode'])
        # print(f"STARTING ANIME FROM EPISODE {anime_history['episode']} {anime_history['time']}")
    
        watching_ep = int(anime_history['episode'])
else: # if there is no history
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

# os.system("./scripts/episode_url.sh")
run_script("episode_url")

# Print the result
if media_id:
    # print(f"Anime ID: {media_id}")
    pass
else:
    print("Anime not found.")
    exit(1)

links = load_links("scripts/tmp/links")

while True:

    try:
        salt = random.randint(0,1500)
        # print("SALT IS:"+str(salt))
        start_video(links[0][1], salt, mpv_args)
        mpv_socket_path = "/tmp/mpvsocket"+str(salt)
        connect_mpv_command = """echo '{ "command": ["get_property", "playback-time"] }' | socat - """+mpv_socket_path
        mpv_pos_command = """echo '{ "command": ["get_property", "time-pos"] }' | socat - """+mpv_socket_path
        mpv_duration_command = """echo '{ "command": ["get_property", "duration"] }' | socat - """+mpv_socket_path

        while True:
            time.sleep(2)
            result = subprocess.run(connect_mpv_command, shell=True, capture_output=True, text=True)
            # print(result)
            if result.returncode == 0:
                output = result.stdout.strip()

                if not output:  # Check if output is empty
                    print("No data received. Retrying...")
                try:
                    data = json.loads(output)
                    if data["error"] == "success":
                        playback_time = round(int(data["data"]), 2)
                        update_anime(get_userconfig_value(user_config, 'history_file'), str(media_id), str(get_contents_of("id")), str(watching_ep), str(playback_time), str(duration), str(title))
                        # print("Playback time:", playback_time)
                        watched_percentage = get_percentage_watched(mpv_socket_path)
                        mpv_playback_speed = get_mpv_playback_speed(mpv_socket_path)
                        # print("mpv speed", mpv_playback_speed)
                        # print("watched percentage", watched_percentage)
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
    except ConnectionRefusedError: # doesnt work ig
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

    print("binge watching")
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
        print(f"Starting next episode: {current_ep}")
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
        run_script("episode_url")
        links = load_links("scripts/tmp/links")
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