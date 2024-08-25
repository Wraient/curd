
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


access_token = os.environ.get('ANILIST_ACCESS_TOKEN')
user_id = os.environ.get('ANILIST_USER_ID')

# print(access_token)
# print(user_id)

def run_script(script):
    os.system(f"./scripts/{script}.sh")

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

# Remove everything from the last '(' including the parentheses


# os.system("./scripts/episode_list.sh")
run_script("episode_list")

# with open("scripts/tmp/episode_list") as ep_list:
#     temp = ep_list.read()
#     temp = temp.split()
#     last_episode = temp[-1]
# ep_no = input(f"Enter episode number: (number of episodes: {last_episode})\n")

while True:
    with open("scripts/tmp/ep_no", "w") as ep_no_file:
        with open("scripts/tmp/episode_list") as ep_list:
            temp = ep_list.read()
            temp = temp.split()
            last_episode = int(temp[-1])

        if progress == last_episode:
            user_input = input("Do you want to start this anime from beginning? (y/n):")
            if user_input.lower() == "yes" or user_input.lower() == "y" or user_input.lower() == "":
                progress = 0
            else:
                progress = last_episode - 1

        # print(progress)
        ep_no_file.write(str(progress+1))

    os.system("./scripts/episode_url.sh")

    # Print the result
    if anime_id:
        print(f"Anime ID: {anime_id}")
    else:
        print("Anime not found.")

    # with open("scripts/tmp/link", 'r') as links:
    #     link = links.read()

    links = load_links("scripts/tmp/links")

    try:
        salt = random.randint(0,500)
        print("SALT IS:"+str(salt))
        start_video(links[0][1], salt)
        # print('test')
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
                        print("Playback time:", playback_time)
                    else:
                        print("Error:", data["error"])
                        break

                except json.decoder.JSONDecodeError as e:
                    print("Error decoding JSON:", e)
                    break  # Exit the loop on unexpected JSON error
            else:
                print("Error:", result.stderr)
                break

    # break
    # Define the command to run

    # Execute the command and capture the output
    # result = subprocess.run(command, capture_output=True, text=True)

    except ConnectionRefusedError:
        print("Player Closed")
    finally:
        to_continue_or_not_to_continue = input("Continue? (y/n)\n")
        if to_continue_or_not_to_continue.lower() == "yes" or to_continue_or_not_to_continue.lower() == "y" or to_continue_or_not_to_continue == "":
            pass
        else:
            break
# except:
#     pass