# Curd

A cli application to stream anime with [Anilist](https://anilist.co/) and [MyAnimeList](https://myanimelist.net/) integration and Discord RPC written in golang.
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
- **Dual Provider Support**: Choose between [Anilist](https://anilist.co/) or [MyAnimeList (MAL)](https://myanimelist.net/) to track your anime
- Update anime progress automatically on your chosen provider after completion
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

# For x86_64 systems:
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd-linux-x86_64

# For ARM64 systems:
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd-linux-arm64

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

# For x86_64 systems:
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd-linux-x86_64

# For ARM64 systems:
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd-linux-arm64

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

# For x86_64 systems:
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd-linux-x86_64

# For ARM64 systems:
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd-linux-arm64

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

- For Universal Binary (works on both architectures):
```bash
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd-macos-universal
```

Then complete the installation:

```bash
chmod +x curd
sudo mv curd /usr/local/bin/
curd
```

</details>

<details>
<summary>Windows Installation</summary>

Option 1: Using the installer
- Download and run the [Windows Installer](https://github.com/Wraient/curd/releases/latest/download/curd-windows-installer.exe)

Option 2: Standalone executable
- Download [curd-windows-x86_64.exe](https://github.com/Wraient/curd/releases/latest/download/curd-windows-x86_64.exe)
</details>

<details>
<summary>Generic Installation</summary>

Choose the appropriate binary for your system:
```bash
# For Linux x86_64:
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd-linux-x86_64

# For Linux ARM64:
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd-linux-arm64

# For macOS Universal:
curl -Lo curd https://github.com/Wraient/curd/releases/latest/download/curd-macos-universal

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

## Data Storage

<details>
<summary>Windows</summary>
Stroage: (Token, Timestamps, debug.log, etc) 

```bash
C:\.local\share\curd
```

Config : 

```bash
C:\Users\USERNAME\AppData\Roaming\Curd
```

</details>

<details>
<summary>Linux/Unix</summary>
Stroage: (Token, Timestamps, debug.log, etc)

```bash
$USER/.local/share/curd
```

Config : 

```bash
$USER/.config/curd
```

</details>

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

## Using MyAnimeList (MAL) with Curd

Curd supports both AniList and MyAnimeList as anime tracking providers. Here's a comprehensive step-by-step guide to set up and use MAL with Curd.

### Quick Overview

**What you'll need:**
- A MyAnimeList account
- MAL API Client credentials (Client ID and Client Secret)
- 5-10 minutes for setup

**Setup Steps:**
1. Create a MAL API Client → Get Client ID and Secret
2. Configure Curd → Add credentials to config file
3. Authenticate → Login via OAuth
4. Start watching → Enjoy automatic progress tracking!

---

### Step 1: Create a MAL API Client

Before you can use Curd with MyAnimeList, you need to create a MAL API client to get your Client ID and Client Secret.

1. **Sign in to MyAnimeList**
   - Go to [MyAnimeList](https://myanimelist.net/) and log in to your account
   - If you don't have an account, create one first

2. **Access API Settings**
   - Navigate to [MAL API Client Registration](https://myanimelist.net/apiconfig)
   - Or go to: Profile → Account Settings → API

3. **Create New Client**
   - Click on "Create ID" or "Add New Client"
   - Fill in the required information:
     - **App Name**: `Curd` (or any name you prefer)
     - **App Type**: Select `web`
     - **App Description**: `Curd is a anime streaming CLI application that I use to watch anime and track my progress on MyAnimeList.`
     - **App Redirect URL**: `http://localhost:8080/callback`
     - **Homepage URL**: `https://github.com/Wraient/curd` (optional)
     - **Commercial/Non-Commercial**: Select `non-commercial`

4. **Save Your Credentials**
   - After creating, you'll receive:
     - **Client ID**: A string of characters (save this)
     - **Client Secret**: Another string of characters (save this securely)
   - **Important**: Keep these credentials private and secure!

### Step 2: Configure Curd to Use MAL

1. **Edit the Curd Configuration File**
   ```bash
   curd -e
   ```

   Or manually open the config file:
   - **Linux/macOS**: `~/.config/curd/curd.conf`
   - **Windows**: `C:\Users\USERNAME\AppData\Roaming\Curd\curd.conf`

2. **Update the Following Settings**
   ```conf
   AnimeProvider = mal
   MALClientID = your_client_id_here
   MALClientSecret = your_client_secret_here
   ```

   Replace `your_client_id_here` and `your_client_secret_here` with the credentials you saved from Step 1.

3. **Save the Configuration File**

### Step 3: Authenticate with MAL

1. **Run Curd for the First Time**
   ```bash
   curd
   ```

2. **OAuth Authentication Flow**
   - Curd will automatically open your default web browser
   - You'll be redirected to MyAnimeList's authorization page
   - Click "Allow" or "Authorize" to grant Curd access to your MAL account

3. **Complete Authentication**
   - After authorization, you'll be redirected to a callback page
   - The authentication token will be saved automatically
   - You can close the browser window

4. **Token Storage**
   - Your MAL token is securely stored at:
     - **Linux/macOS**: `~/.local/share/curd/mal_token.json`
     - **Windows**: `C:\.local\share\curd\mal_token.json`

### Step 4: Start Using Curd with MAL

Once authenticated, Curd will:
- Display your MAL anime lists (Watching, Completed, On Hold, Dropped, Plan to Watch)
- Automatically update your MAL progress as you watch episodes
- Sync your scores and status changes
- Allow you to add new anime to your MAL lists

**Example Usage:**
```bash
# Start watching from your Currently Watching list
curd

# Continue your last watched anime
curd -c

# Add a new anime to your MAL list
curd -new

# Watch with rofi interface
curd -rofi
```

### Step 5: Managing Your MAL Integration

#### Switching Back to AniList
If you want to switch back to AniList:
1. Edit the config: `curd -e`
2. Change: `AnimeProvider = anilist`
3. Save and restart Curd

#### Re-authenticating MAL
If you need to re-authenticate (token expired, changed accounts, etc.):
1. Delete the token file:
   ```bash
   # Linux/macOS
   rm ~/.local/share/curd/mal_token.json

   # Windows
   del C:\.local\share\curd\mal_token.json
   ```
2. Run `curd` again to start the authentication process

#### Viewing Your Token
Your MAL authentication token is stored in JSON format at the token file location. It includes:
- Access token
- Refresh token
- Token expiration time

### Troubleshooting

**Problem**: "Failed to get MAL user info" or authentication errors

**Solutions**:
1. Verify your Client ID and Client Secret in the config file
2. Ensure the redirect URI in your MAL API settings matches: `http://localhost:8080/callback`
3. Delete the token file and re-authenticate
4. Check that port 8080 is not blocked by a firewall

**Problem**: Anime not updating on MAL

**Solutions**:
1. Check your internet connection
2. Verify the token hasn't expired (re-authenticate if needed)
3. Check the debug log at: `~/.local/share/curd/debug.log` (Linux/macOS) or `C:\.local\share\curd\debug.log` (Windows)

**Problem**: Can't find anime when searching

**Solutions**:
1. Try searching with the English title instead of Romaji
2. Use alternative titles or abbreviations
3. The anime might not be in MAL's database yet

### MAL vs AniList Comparison

| Feature | AniList | MyAnimeList |
|---------|---------|-------------|
| Anime Coverage | Extensive | Extensive |
| Scoring System | 0-100 | 0-10 |
| Image Previews in Rofi | Yes | Yes |
| Progress Tracking | Yes | Yes |
| Status Categories | 6 categories | 5 categories |
| OAuth Setup | Simple | Requires API Client |

### Additional Notes

- MAL uses a 0-10 scoring system, while AniList uses 0-100
- When switching providers, your local watch history remains intact
- Both providers work with all Curd features (Discord RPC, skip features, etc.)
- You can maintain lists on both platforms by switching the provider setting

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
| `AnimeProvider`           | Enum       | `anilist`, `mal`                          | Specifies which anime tracking service to use (AniList or MyAnimeList).                          |
| `MALClientID`             | String     | Your MAL Client ID                        | Client ID from MyAnimeList API (required when using MAL as provider).                            |
| `MALClientSecret`         | String     | Your MAL Client Secret                    | Client Secret from MyAnimeList API (required when using MAL as provider).                        |

## Todo (fix)
- Use Powershell for windows token input instead of notepad or cmd
- Find a way to get "Watching" instead of "Playing" Activity type in discord rpc (Implemented in [Dantotsu](https://github.com/aayush2622/Dartotsu) and [Premid](https://github.com/PreMiD/PreMiD))
- Add a better way to do commands in windows (Convinience for users)

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
