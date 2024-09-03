
# search (anime_list.sh) -> tmp/anime_list 
# anime_id -> episode_list.sh
# episode_number -> episdoe_url.sh -> tmp/links 

import re
import os
import json
import socket
import random
import subprocess
import time

from anilist import search_anime_by_title
from anilist import get_user_data
from select_link import load_links
from start_video import *
from select_anime import load_anime_data
from select_anime import extract_anime_info
from select_anime import select_anime

from track_anime import add_anime, update_anime, get_all_anime, delete_anime

access_token = os.environ.get('ANILIST_ACCESS_TOKEN')
user_id = os.environ.get('ANILIST_USER_ID')

# print(access_token)
# print(user_id)

def get_contents_of(tmp_file_name):
    with open(f"scripts/tmp/{tmp_file_name}", "r") as temp_file:
        return temp_file.read()

def run_script(script):
    os.system(f"./scripts/{script}.sh")

def load_config():
    command = """echo $(xdg-user-dir CONFIG)"""

    result = subprocess.run(command, shell=True, capture_output=True, text=True)
    output = result.stdout.strip()

    folder_name = f"{output}/.config/curd"
    file_name = "curd.conf"

    config_file_path = os.path.join(folder_name, file_name)

    print(config_file_path)
    if not os.path.exists(os.path.join(config_file_path)):
        print("Creating config")
        
        try:
            print("making folder")
            os.makedirs(folder_name)
        except:
            print("error making folder")
            pass

        with open(config_file_path, 'w') as file:
            file.write(r"""player="mpv"
show_adult_content=false
history_file="$HOME/.local/share/curd/curd_history.txt"
subs_language="english"
sub_or_dub="sub"
score_on_completion=true
discord_presence=true
presence_script_path="curddiscordpresence.py"
""")
            
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

user_config = load_config() # python dictionary containing all the configs as key value pairs

print("downloading user data")
anilist_user_data = get_user_data(access_token, user_id)
with open("response.json", "w") as response:
    response.write(str(anilist_user_data))
try:
    if anilist_user_data['data'] == None:
        print("Cannot process user data.")
        print(anilist_user_data)
        exit()
except:
    pass

# Searching for anime

# with open("scripts/tmp/query", "w") as file:
#     file.write(input("Enter the anime you want:\n"))

# os.system("./scripts/anime_list.sh")

# with open('response.json', 'r') as file:
    # data = json.load(file)

# print(data)
# print(type(data['data']))

anime_dict = extract_anime_info(anilist_user_data)[0]
# print(anime_dict)
# anime_dict = load_anime_data("scripts/tmp/anime_list")
# print(anime_dict)
# print(f"ANIME DICT = {anime_dict}")

select_anime(anime_dict)

with open("scripts/tmp/anime", "r") as anime:
    anime_name = anime.read()

with open("scripts/tmp/query", "w") as query_file:
    query_file.write(anime_name)

run_script("anime_list")

anime_dict = load_anime_data("scripts/tmp/anime_list")

cleaned_text = re.sub(r'\(.*$', '', anime_name).strip()

try:
    result = search_anime_by_title(anilist_user_data, cleaned_text)[0]
    print(result)
    anime_id = result['id']
    progress = int(result['progress'])
    title = result['english_title']
    total_episodes = result['episodes']
except:
    print("fucked up")
    anime_id = 98460
    progress = 0

try:
    print("Trying to automate")
    with open("scripts/tmp/id", "w") as id_file:
        id_file.write(anime_dict[anime_name+f" ({total_episodes} episodes)"])
    with open("scripts/tmp/anime", "w") as anime_name_file:
        anime_name_file.write(anime_name)
except:
    print("Cannot automate")
    select_anime(anime_dict)

run_script("episode_list")

# with open("scripts/tmp/episode_list") as ep_list:
#     temp = ep_list.read()
#     temp = temp.split()
#     last_episode = temp[-1]
# ep_no = input(f"Enter episode number: (number of episodes: {last_episode})\n")

binge_watching = False

while True:
    with open("scripts/tmp/episode_list") as ep_list:
        temp = ep_list.read()
        temp = temp.split()
        last_episode = int(temp[-1])

    if binge_watching:
        print("binge watching")
        with open("scripts/tmp/ep_no", "r") as ep_no_file_1:
            current_ep = int(ep_no_file_1.read())
        if current_ep == last_episode:
            print("completed anime.")
            # TODO: Add way to rate the anime
            delete_anime(user_config['history_file'], anime_id, get_contents_of("id"))
            exit(0)

        else:
            print(f"Next episode is here, goshujin sama progress: {current_ep} {type(current_ep)}")
            with open("scripts/tmp/ep_no", "w") as ep_no_file:
                ep_no_file.write(str(int(current_ep)+1))
        
        watching_ep = current_ep+1
    else:
        print("First time watch")
        with open("scripts/tmp/ep_no", "w") as ep_no_file:

            if progress == last_episode:
                user_input = input("Do you want to start this anime from beginning? (y/n):")
                if user_input.lower() == "yes" or user_input.lower() == "y" or user_input.lower() == "":
                    progress = 0
                else:
                    print("Starting last episode of anime")
                    progress = last_episode - 1

            watching_ep = progress+1
            ep_no_file.write(str(watching_ep))

    os.system("./scripts/episode_url.sh")

    # Print the result
    if anime_id:
        print(f"Anime ID: {anime_id}")
    else:
        print("Anime not found.")

    links = load_links("scripts/tmp/links")

    try:
        salt = random.randint(0,500)
        print("SALT IS:"+str(salt))
        start_video(links[0][1], salt)
        command = """echo '{ "command": ["get_property", "playback-time"] }' | socat - /tmp/mpvsocket"""+str(salt)

        while True:
            time.sleep(2)

            result = subprocess.run(command, shell=True, capture_output=True, text=True)
            # print(result)
            if result.returncode == 0:
                output = result.stdout.strip()

                if not output:  # Check if output is empty
                    print("No data received. Retrying...")
                try:
                    data = json.loads(output)
                    if data["error"] == "success":
                        playback_time = round(int(data["data"]))

                        print(user_config['history_file'])

                        update_anime(user_config['history_file'], str(anime_id), str(get_contents_of("id")), str(watching_ep), str(playback_time), str(title))
                        print("Playback time:", playback_time)
                    else:
                        print("Error:", data["error"])
                        break

                except json.decoder.JSONDecodeError as e:
                    print("Error decoding JSON:", e)
                    break  # Exit the loop on unexpected JSON error
            else:
                killing_error = str(result.stderr)
                if killing_error == "property unavailable": # mpv is not started yet
                    pass
                else:
                    print("Error:", killing_error)
                    break
    except ConnectionRefusedError:
        print("Player Closed")
    finally:
        to_continue_or_not_to_continue = input("Continue? (y/n)\n")
        if to_continue_or_not_to_continue.lower() == "yes" or to_continue_or_not_to_continue.lower() == "y" or to_continue_or_not_to_continue == "":
            binge_watching = True
            # ep_no_file
        else:
            break
