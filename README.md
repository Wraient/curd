
# Curd

A cli application to stream anime with [Anilist](https://anilist.co/) integration and Discord RPC written in golang.

Note: This currently only works for linux.

## Demo Video
https://github.com/user-attachments/assets/3b9578aa-396a-4313-8254-d0954041d6ba

## Features
- Stream anime online
- Update anime in Anilist after completion
- Skip anime Intro and Outro
- Skip Filler and Recap episodes
- Discord RPC about the anime
- Local anime history to continue from where you left off last time
- Save mpv speed for next episode
- Configurable through config file

## Installing and Setup
### Linux
<details>
<summary>Arch Linux / Manjaro (AUR-based systems)</summary>


Using Yay

```
yay -Sy curd
```

or using Paru:

```
paru -Sy curd
```

Or manually:

```
git clone https://aur.archlinux.org/curd.git
cd curd
makepkg -si
```
</details>

<details>
<summary>Debian / Ubuntu (and derivatives)</summary>

```
sudo apt update
sudo apt install mpv curl
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd
chmod +x curd
sudo mv curd /usr/local/bin/
curd
```
</details>

<details>
<summary>Fedora</summary>

```
sudo dnf update
sudo dnf install mpv curl
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd
chmod +x curd
sudo mv curd /usr/local/bin/
curd
```
</details>

<details>
<summary>openSUSE</summary>

```
sudo zypper refresh
sudo zypper install mpv curl
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd
chmod +x curd
sudo mv curd /usr/local/bin/
curd
```
</details>

<details>
<summary>Other Linux Distributions (Generic Instructions)</summary>

```
# Install mpv and curl

curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd
chmod +x curd
sudo mv curd /usr/local/bin/
curd
```
</details>

<details>
<summary>Uninstallation</summary>

```
sudo rm /usr/local/bin/curd
```

For AUR-based distributions:

```
yay -R curd
```
</details>



## Usage

- For first time, just run the script and go to the link and enter your anilist token. After that you can use the script to watch anime.

|Description            | Command          |
------------------------|------------------
|*Watch dub*            | `curd -dub`      |
|*Watch sub*            | `curd -sub`      |
|*Update the script*    | `curd -u`        |
|*Edit config file*    | `curd -e`        |
|*Continue last watching anime* |`curd -c`  |
|*Help*                 | `curd -help`     |


Script is made in a way that you use it for one session of watching.

You can quit it anytime and the resume time would be saved in the history file

more settings can be found at config file.
config file is located at ```~/.config/curd/curd.conf```

## Dependencies
- mpv - Video player (vlc support might be added later)
- Socat - Receive output of mpv commands
- Pypresence - Discord RPC
    
## API Used
- [Anilist API](https://anilist.gitbook.io/anilist-apiv2-docs) - Update user data and download user data
- [AniSkip API](https://api.aniskip.com/api-docs) - Get anime intro and outro timings
- [AllAnime Content](https://allanime.to/) - Fetch anime url
- [Jikan](https://jikan.moe/) - Get filler episode number

## Credits
- [ani-cli](https://github.com/pystardust/ani-cli) - Code for fetching anime url
- [jerry](https://github.com/justchokingaround/jerry) - For the inspiration
