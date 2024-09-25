
# Curd

A cli application to stream anime with [Anilist](https://anilist.co/) integration and Discord RPC.

Note: This currently only works for linux.

## Features
- Stream anime online
- Update anime in Anilist after completion
- Skip anime Intro and Outro
- Discord RPC about the anime
- Local anime history to continue from where you left off last time
- Save mpv speed for next episode
- Configurable through config file

## Installing and Setup
### Linux
<details><summary>Debian</summary>
  

```
    sudo apt-get install socat
    pip3 install pypresence requests
    git clone https://github.com/wraient/curd --depth=1
    python3 ./curd/curd.py
```

</details>

<details><summary>Arch Linux</summary>
  
```
sudo pacman -Sy socat
git clone https://github.com/wraient/curd --depth=1
cd ./curd
python -m venv venv
source ./venv/bin/activate
pip3 install pypresence requests
python3 curd.py
```
</details>


## Usage

- For first time, just run the script and go to the link and enter your anilist token. After that you can use the script to watch anime.

|Description            | Command          |
------------------------|------------------
|*Watching new anime*   | ` curd -new`     |
|*Watch dub*            | `curd -dub`      |
|*Watch sub*            | `curd -sub`      |
|*Help*                 | `curd -help`     |

Script is made in a way that you use it for one session of watching.

You can quit it anytime and the resume time would be saved in the history file

more settings can be found at config file.
config file is located at ```~/.config/curd/curd.conf```

**Help**
    
    
## API Used
- [Anilist API](https://anilist.gitbook.io/anilist-apiv2-docs) - To update user data and download user data
- [AniSkip API](https://api.aniskip.com/api-docs) - To get anime intro and outro timings
- [AllAnime Content](https://allanime.to/) - To fetch anime url

## Credits
- [ani-cli](https://github.com/pystardust/ani-cli) - Code for fetching anime url
- [jerry](https://github.com/justchokingaround/jerry) - For the inspiration
