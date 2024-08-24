
# search (anime_list.sh) -> tmp/anime_list 
# anime_id -> episode_list.sh
# episode_number -> episdoe_url.sh -> tmp/links 

import re
import os
import json
from anilist import search_anime_by_title

with open("scripts/tmp/query", "w") as file:
    file.write(input("Enter the anime you want:\n"))

os.system("./scripts/anime_list.sh")

import select_anime

with open("scripts/tmp/anime", "r") as anime:
    anime_name = anime.read()

with open('response.json', 'r') as file:
    data = json.load(file)

# Remove everything from the last '(' including the parentheses
cleaned_text = re.sub(r'\(.*$', '', anime_name).strip()

# try:
result = search_anime_by_title(data, cleaned_text)[0]
print(result)
anime_id = result['id']
progress = int(result['progress'])
title = result['english_title']
# except:
    # print("fucked up")
    # anime_id = 98460


os.system("./scripts/episode_list.sh")

# with open("scripts/tmp/episode_list") as ep_list:
#     temp = ep_list.read()
#     temp = temp.split()
#     last_episode = temp[-1]
# ep_no = input(f"Enter episode number: (number of episodes: {last_episode})\n")

os.system("./scripts/episode_url.sh")


# Define the title to search for
# search_title = ''  # Replace with the actual title you're searching for

with open("scripts/tmp/ep_no", "w") as ep_no_file:
    # ep_no.write("1")
    # ep_no_file.write(ep_no)
    ep_no_file.write(str(progress))

# Search for the anime and get its ID

# Print the result
if anime_id:
    print(f"Anime ID: {anime_id}")
else:
    print("Anime not found.")


