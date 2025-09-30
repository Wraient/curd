# MyAnimeList (MAL) Integration Guide for Curd

## Overview

Curd now supports **MyAnimeList** as an alternative to AniList! You can switch between providers and authenticate with your MAL account.

## Prerequisites

### 1. Create a MAL API Application

Before using MAL with Curd, you need to register your application:

1. Go to https://myanimelist.net/apiconfig
2. Click **"Create ID"**
3. Fill in the form:
   - **App Name**: `Curd` (or any name you prefer)
   - **App Type**: Choose `web` or `other`
   - **Description**: `CLI anime tracker`
   - **Redirect URL**: `http://localhost:8080/callback`
   - **Homepage URL**: Can leave empty or use `https://github.com/wraient/curd`
4. Click **"Submit"**
5. **Copy your Client ID** - you'll need this later

## Setup Instructions

### Step 1: Configure MAL Provider

Edit your Curd config file:
```bash
./curd -e
```

Or manually edit: `~/.config/curd/curd.conf`

Add or modify these lines:
```
AnimeProvider=mal
MALClientID=YOUR_CLIENT_ID_HERE
```

Replace `YOUR_CLIENT_ID_HERE` with the Client ID you got from MAL.

### Step 2: Authenticate with MAL

Run the authentication command:
```bash
./curd -change-token
```

This will:
1. Open your browser automatically
2. Ask you to log in to MyAnimeList
3. Request permission to access your anime list
4. Redirect back to Curd with your token
5. Save the token for future use

### Step 3: Start Using Curd with MAL

Now you can use Curd normally:
```bash
./curd                    # Browse and watch your anime
./curd -new              # Add new anime to your MAL list
./curd -c                # Continue last watched anime
```

## Features Supported with MAL

- ✅ View your anime lists (watching, completed, on hold, dropped, plan to watch)
- ✅ Search for anime on MAL
- ✅ Update watch progress automatically
- ✅ Add anime to your list
- ✅ Update anime status (watching → completed, etc.)
- ✅ Rate anime on completion
- ✅ Track watch history locally

## Switching Between Providers

You can easily switch between AniList and MAL:

### Switch to MAL:
```bash
# Edit config
./curd -e

# Change these lines:
AnimeProvider=mal
MALClientID=YOUR_MAL_CLIENT_ID

# Re-authenticate
./curd -change-token
```

### Switch back to AniList:
```bash
# Edit config
./curd -e

# Change this line:
AnimeProvider=anilist

# Re-authenticate (if needed)
./curd -change-token
```

## Configuration Reference

| Config Option | Values | Description |
|--------------|--------|-------------|
| `AnimeProvider` | `anilist` or `mal` | Which service to use for anime tracking |
| `MALClientID` | Your Client ID | Required when using MAL provider |

## Token Storage

Tokens are stored separately for each provider:
- **AniList**: `~/.local/share/curd/anilist_token.json`
- **MAL**: `~/.local/share/curd/mal_token.json`

You can switch providers without losing your authentication for either service.

## Troubleshooting

### "MAL Client ID not configured"
- Make sure you set `MALClientID` in your config file
- Run `./curd -e` to edit the config

### Authentication fails
- Check that your Redirect URL in MAL API config is exactly: `http://localhost:8080/callback`
- Make sure port 8080 is not already in use
- Try running `./curd -change-token` again

### "Failed to get user ID"
- Your token may have expired
- Run `./curd -change-token` to re-authenticate

### Can't find my anime
- MAL's search uses different anime IDs than AniList
- Try searching with the exact title or alternative titles
- Some anime may have different names on MAL

## Differences Between AniList and MAL

| Feature | AniList | MAL |
|---------|---------|-----|
| Episode duration | Fetched from API | Default 24 min |
| Cover images | Full support | Full support |
| Rewatching | ✅ Supported | ✅ Supported |
| Scoring | 0-10 (float) | 0-10 (integer) |
| Status names | CURRENT, COMPLETED, etc. | watching, completed, etc. |

Note: Both providers work identically within Curd thanks to the abstraction layer.

## Example Workflow

```bash
# Initial setup
cd /home/badhon/Documents/curd-MAL
go build -o curd ./cmd/curd

# Configure for MAL
./curd -e
# Set: AnimeProvider=mal
# Set: MALClientID=your_client_id

# Authenticate
./curd -change-token

# Add new anime
./curd -new
# Search: "One Piece"
# Select from results
# Choose "Currently Watching"

# Watch anime
./curd
# Select category (or use -current flag)
# Select anime
# Enjoy! Progress is tracked automatically

# Continue later
./curd -c
```

## Advanced Usage

### Test MAL authentication only:
```bash
# Create a test file
cat > test_mal.go << 'EOF'
package main

import (
	"fmt"
	"github.com/wraient/curd/internal"
)

func main() {
	config := internal.CurdConfig{
		AnimeProvider: "mal",
		MALClientID:   "YOUR_CLIENT_ID",
		StoragePath:   "$HOME/.local/share/curd",
	}

	var user internal.User
	internal.ChangeToken(&config, &user)

	fmt.Println("Token:", user.Token)

	userID, username, _ := internal.GetProviderUserID(&config, user.Token)
	fmt.Printf("Logged in as: %s (ID: %d)\n", username, userID)
}
EOF

go run test_mal.go
```

## Support

If you encounter issues:
1. Check the debug log: `~/.local/share/curd/debug.log`
2. Report issues at: https://github.com/Wraient/curd/issues
3. Join Discord: https://discord.gg/cNaNVEE3B6

## Credits

- MAL API Documentation: https://myanimelist.net/apiconfig/references/api/v2
- Original Curd project: https://github.com/Wraient/curd