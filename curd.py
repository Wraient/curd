
# search (anime_list.sh) -> tmp/anime_list 
# anime_id -> episode_list.sh
# episode_number -> episdoe_url.sh -> tmp/links 

import os
# from select_anime import main as select

with open("scripts/tmp/query", "w") as file:
    file.write(input("Enter the anime you want:\n"))

os.system("./scripts/anime_list.sh")

# with open("scripts/tmp/anime_list", "r") as anime_list:
#     anime_id = str(anime_list.read()).split()[0]

import select_anime
# try:
#     curses.wrapper(select)
# except:
#     pass

# with open("scripts/tmp/id", "w") as id:
#     id.write(anime_id)

os.system("./scripts/episode_list.sh")

with open("scripts/tmp/episode_list") as ep_list:
    temp = ep_list.read()
    temp = temp.split()
    last_episode = temp[-1]
ep_no = input(f"Enter episode number: (number of episodes: {last_episode})\n")

with open("scripts/tmp/ep_no", "w") as ep_no_file:
    # ep_no.write("1")
    ep_no_file.write(ep_no)

os.system("./scripts/episode_url.sh")

