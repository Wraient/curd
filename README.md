
# Curd

A cli application to stream anime with [Anilist](https://anilist.co/) integration and Discord RPC written in golang.
Works on Windows and Linux

## Join the discord server

https://discord.gg/cNaNVEE3B6

## Demo Video
Normal mode:


https://github.com/user-attachments/assets/376e7580-b1af-40ee-82c3-154191f75b79

Rofi with Image preview


https://github.com/user-attachments/assets/cbf799bc-9fdd-4402-ab61-b4e31f1e264d


## Features
- Stream anime online
- Update anime in Anilist after completion
- Skip anime Intro and Outro
- Skip Filler and Recap episodes
- Discord RPC about the anime
- Rofi support
- Image preview in rofi
- Local anime history to continue from where you left off last time
- Save mpv speed for next episode
- Configurable through config file


## Installing and Setup
> **Note**: `Curd` requires `mpv`, `rofi`, and `ueberzugpp` for Rofi support and image preview. These are included in the installation instructions below for each distribution.

### Linux
<details>
<summary>Arch Linux / Manjaro (AUR-based systems)</summary>

Using Yay:

```bash
yay -Sy curd
```

or using Paru:

```bash
paru -Sy curd
```

Or, to manually clone and install:

```bash
git clone https://aur.archlinux.org/curd.git
cd curd
makepkg -si
sudo pacman -S rofi ueberzugpp
```
</details>

<details>
<summary>Debian / Ubuntu (and derivatives)</summary>

```bash
sudo apt update
sudo apt install mpv curl rofi ueberzugpp
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd
chmod +x curd
sudo mv curd /usr/bin/
curd
```
</details>

<details>
<summary>Fedora Installation</summary>

```bash
sudo dnf update
sudo dnf install mpv curl rofi ueberzugpp
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd
chmod +x curd
sudo mv curd /usr/bin/
curd
```
</details>

<details>
<summary>openSUSE Installation</summary>

```bash
sudo zypper refresh
sudo zypper install mpv curl rofi ueberzugpp
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd
chmod +x curd
sudo mv curd /usr/bin/
curd
```
</details>

<details>
<summary>NixOS Installation</summary>

1. Add curd as a flake input, for example:
```nix
{
    inputs = {
        nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
        curd = {
            url = "github:Wraient/curd";
            inputs.nixpkgs.follows = "nixpkgs";
        };
    }
}
```
2. Install the package, for example:
```nix
{inputs, pkgs, ...}: {
  environment.systemPackages = [
    inputs.curd.packages.${pkgs.system}.default
  ];
}
```

</details>

<details>
<summary>macOS Installation</summary>

Install required dependencies
```bash
brew install mpv curl
```

Download the appropriate binary for your system:

- For Apple Silicon (M1/M2) Macs:
```bash
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd-macos-arm64 
```

- For Intel Macs:
```bash
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd-macos-x86_64
```

Then complete the installation:

```bash
chmod +x curd
sudo mv curd /usr/local/bin/
curd
```

</details>

<details>
<summary>Generic Installation</summary>

```bash
# Install mpv, curl, rofi, and ueberzugpp (required for image preview)
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd
chmod +x curd
sudo mv curd /usr/bin/
curd
```
</details>

<details>
<summary>Uninstallation</summary>

```bash
sudo rm /usr/bin/curd
```

For AUR-based distributions:

```bash
yay -R curd
```
</details>

### [Windows Installer](https://github.com/Wraient/curd/releases/latest/download/curd-windows-installer.exe)

## Usage

Run `curd` with the following options:

```bash
curd [options]
```

### Arguments would always take precedence over configuration

> **Note**:
> - To use rofi you need rofi and ueberzug installed.
> - Rofi .rasi files are at default `~/.local/share/curd/`
> - You can edit them as you like.
> - If there are no rasi files with specific names, they would be downloaded from this repo.


### Options

| Flag                      | Description                                                             | Default       |
|---------------------------|-------------------------------------------------------------------------|---------------|
| `-c`                      | Continue the last episode                                              | -             |
| `-change-token`           | Change your authentication token                                       | -             |
| `-dub`                    | Watch the dubbed version of the anime                                  | -             |
| `-sub`                    | Watch the subbed version of the anime                                  | -             |
| `-new`                    | Add a new anime to your list                                           | -             |
| `-e`                      | Edit the configuration file                                            | -             |
| `-skip-op`                | Automatically skip the opening section of each episode                 | `true`        |
| `-skip-ed`                | Automatically skip the ending section of each episode                  | `true`        |
| `-skip-filler`            | Automatically skip filler episodes                                     | `true`        |
| `-skip-recap`             | Automatically skip recap sections                                      | `true`        |
| `-discord-presence`       | Enable or disable Discord presence                                     | `true`        |
| `-image-preview`          | Show an image preview of the anime                                     | -             |
| `-no-image-preview`       | Disable image preview                                                  | -             |
| `-next-episode-prompt`    | Prompt for the next episode after completing one                       | -             |
| `-rofi`                   | Open anime selection in the rofi interface                             | -             |
| `-no-rofi`                | Disable rofi interface                                                 | -             |
| `-percentage-to-mark-complete` | Set the percentage watched to mark an episode as complete       | `85`          |
| `-player`                 | Specify the player to use for playback                                 | `"mpv"`       |
| `-save-mpv-speed`         | Save the current MPV speed setting for future sessions                 | `true`        |
| `-score-on-completion`    | Prompt to score the episode on completion                              | `true`        |
| `-storage-path`           | Path to the storage directory                                          | `"$HOME/.local/share/curd"` |
| `-subs-lang`              | Set the language for subtitles                                         | `"english"`   |
| `-u`                      | Update the script                                                      | -             |
| `-v`                      | Show curd version                                                      | -             |

### Examples

- **Continue the Last Episode**:
  ```bash
  curd -c
  ```

- **Add a New Anime**:
  ```bash
  curd -percentage-to-mark-complete=90
  ```

- **Play with Rofi and Image Preview**:
  ```bash
  curd -rofi -image-preview
  ```

## Configuration

All configurations are stored in a file you can edit with the `-e` option.

```bash
curd -e
```

Script is made in a way that you use it for one session of watching.

You can quit it anytime and the resume time would be saved in the history file

more settings can be found at config file.
config file is located at ```~/.config/curd/curd.conf```

| **Option**               | **Type**   | **Valid Values**                           | **Description**                                                                                   |
|---------------------------|------------|-------------------------------------------|---------------------------------------------------------------------------------------------------|
| `DiscordPresence`         | Boolean    | `true`, `false`                           | Enables or disables Discord Rich Presence integration.                                            |
| `AnimeNameLanguage`       | Enum       | `english`, `romaji`                       | Sets the preferred language for anime names.                                                      |
| `MpvArgs`                 | List       | all mpv args eg ["--fullscreen=yes", "--mute=yes"]     | Add args to mpv player                                                               | 
| `AddMissingOptions`       | Boolean    | `true`, `false`                           | Automatically adds missing configuration options with default values to the config file.          |
| `AlternateScreen`         | Boolean    | `true`, `false`                           | Toggles the use of an alternate screen buffer for cleaner UI.                                     |
| `RofiSelection`           | Boolean    | `true`, `false`                           | Enables or disables anime selection via Rofi.                                                     |
| `PercentageToMarkComplete`| Integer    | `0` to `100`                              | Sets the percentage of an episode watched to consider it as completed.                            |
| `StoragePath`             | String     | Any valid path (Environment variables accepted)  | Specifies the directory where Curd stores its data.                                        |
| `SubOrDub`                | Enum       | `sub`, `dub`                              | Sets the preferred format for anime audio.                                                        |
| `NextEpisodePrompt`       | Boolean    | `true`, `false`                           | Prompts the user before automatically playing the next episode.                                   |
| `SubsLanguage`            | String     | `english` (redundant rn)                  | Sets the preferred subtitle language.                                                             |
| `ScoreOnCompletion`       | Boolean    | `true`, `false`                           | Automatically prompts the user to rate the anime upon completion.                                 |
| `SkipOp`                  | Boolean    | `true`, `false`                           | Automatically skips the opening of episodes when supported.                                       |
| `SkipEd`                  | Boolean    | `true`, `false`                           | Automatically skips the ending of episodes when supported.                                        |
| `SkipRecap`               | Boolean    | `true`, `false`                           | Skips recap sections in episodes when supported.                                                  |
| `ImagePreview`            | Boolean    | `true`, `false`                           | Enables or disables image previews during anime selection (only for rofi).                        |
| `Player`                  | String     | `mpv` (redundant rn)                      | Specifies the media player used for streaming or playing anime.                                   |
| `SaveMpvSpeed`            | Boolean    | `true`, `false`                           | Retains the playback speed set in MPV for next episode.                                           |
| `SkipFiller`              | Boolean    | `true`, `false`                           | Skips filler episodes when supported.                                                             |

## Dependencies
- mpv - Video player (vlc support might be added later)
- rofi - Selection menu
- ueberzug - Display images in rofi
    
## API Used
- [Anilist API](https://anilist.gitbook.io/anilist-apiv2-docs) - Update user data and download user data
- [AniSkip API](https://api.aniskip.com/api-docs) - Get anime intro and outro timings
- [AllAnime Content](https://allanime.to/) - Fetch anime url
- [Jikan](https://jikan.moe/) - Get filler episode number

## Credits
- [ani-cli](https://github.com/pystardust/ani-cli) - Code for fetching anime url
- [jerry](https://github.com/justchokingaround/jerry) - For the inspiration
