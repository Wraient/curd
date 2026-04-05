# Android / Termux Support for Curd

## Summary
Add a dedicated Android-on-Termux mode that keeps `curd` terminal-first, uses a separate Android config schema, and introduces an Android playback backend instead of assuming desktop `mpv` process control.

Target outcome:
- Android users run `curd` inside Termux with a mobile-tuned TUI.
- Playback opens in a user-selected Android mpv app or mpv fork package.
- Full watch-flow features are supported only when that player path exposes MPV-compatible IPC/control.
- Desktop config stays clean; Android gets its own config file and only Android-relevant options.

## Implementation Changes
- Add platform detection in startup:
  - Detect Termux/Android from `runtime.GOOS == "android"` or Termux env markers.
  - Default config path on Android to `~/.config/curd/android.conf`.
  - Keep desktop default as `~/.config/curd/curd.conf`.
  - Add `--config` so either platform can override explicitly.

- Split config into shared core plus Android-only schema:
  - Keep shared fields such as `SubOrDub`, `SubsLanguage`, `PercentageToMarkComplete`, `NextEpisodePrompt`, `SkipOp`, `SkipEd`, `SkipFiller`, `SkipRecap`, `ScoreOnCompletion`, `SaveMpvSpeed`, `AnimeNameLanguage`, `MenuOrder`.
  - Remove desktop-only Android noise from desktop config and vice versa by generating different defaults per platform.
  - Android config should add:
    - `AndroidPlayerPackage`
    - `AndroidPlayerActivity`
    - `AndroidPlayerMode` with `ipc` or `intent`
    - `AndroidPlayerIntentAction`
    - `AndroidUseTermuxAPI`
    - `AndroidOpenLinksWith`
    - `AndroidNotifications`
    - `AndroidWakeLock`
    - `AndroidExtraIntentArgs`
  - Android defaults:
    - `RofiSelection=false`
    - `ImagePreview=false`
    - `DiscordPresence=false`
    - `AlternateScreen=false`

- Introduce a playback backend abstraction:
  - Replace direct desktop assumptions in `internal/player.go` with a `PlayerBackend` interface.
  - Desktop backend keeps current MPV IPC behavior.
  - Android backend supports two modes:
    - `ipc`: required for full Android feature parity; launches the selected mpv/mpv fork with a known socket path or equivalent control bridge.
    - `intent`: basic fallback; launches player by Android intent only, with no reliable progress/control.
  - Do not promise full features in `intent` mode.

- Android player launch design:
  - Launch via `am start` using user-configured package/activity, not hardcoded `is.xyz.mpv/.MPVActivity`.
  - Support official mpv-android and mpvEx by config, not by special-casing brand names.
  - Add a player capability check during `curd --android-setup`:
    - Validate package/activity exists.
    - Validate whether IPC/control is available.
    - Write detected capability into Android config.

- Preserve core watch-flow features in Android `ipc` mode:
  - Resume from saved position.
  - Track playback time and duration.
  - Auto update AniList progress.
  - Auto continue to next episode on completion.
  - Skip OP/ED through AniSkip.
  - Skip filler and recap episodes.
  - Save playback speed.
  - Prompt for rating on completion.
  - Sequel prompt / list updates.
  - Continue last episode.
  - Local history.

- Android UI changes:
  - Keep Bubble Tea terminal UI, but add an Android/mobile presentation mode:
    - larger list rows
    - explicit footer shortcuts
    - back behavior mapped cleanly
    - no alternate-screen dependency
    - search-first flow with visible quick actions
  - Do not use rofi or image preview on Android.
  - Use Termux API optionally for:
    - `termux-open-url` for AniList auth and sequel pages
    - `termux-notification` / `termux-toast` for prompts and status
    - `termux-wake-lock` during active playback sessions if enabled

- Android auth and update flow:
  - On Android, prefer `termux-open-url` or Android intent for AniList OAuth instead of generic browser open.
  - Keep localhost callback flow if reachable; retain manual token fallback.
  - Extend `-u` / updater logic in `internal/curd.go` to support an Android release artifact plus a Termux install/update wrapper.
  - First release should support both:
    - direct GitHub Android binary
    - Termux bootstrap/update script that downloads the correct Android asset

## Feature Reality
Features we can support well on Android in `ipc` mode: 12+
- streaming playback
- mobile-tuned TUI selection/search
- continue last
- local resume/history
- AniList tracking/progress updates
- next episode on completion
- skip OP
- skip ED
- skip filler
- skip recap
- save speed
- score-on-completion and sequel prompts
- configurable player package and Android-specific settings
- self-update/install flow

Features that are possible but conditional:
- opening in mpvEx or another fork only if the selected package/activity can be launched reliably and exposes MPV-compatible control
- notifications and wake-locks only when `termux-api` is installed
- seamless same-session player reuse only if Android player integration supports IPC commands like `loadfile`

Features that should be treated as unsupported on Android:
- Discord Rich Presence
- rofi UI
- rofi image preview / ueberzug flow
- full playback control against arbitrary external players that only accept a `VIEW` intent
- guaranteed progress/completion detection if the user picks a player with no IPC/control channel
- desktop-style “one live mpv window” semantics if the Android player does not support file replacement over IPC

## Public Interfaces / CLI Changes
- Add `--config <path>`.
- Add `--android-setup` to:
  - detect Termux API tools
  - probe configured player package/activity
  - verify IPC capability
  - create `android.conf`
- Keep existing flags, but on Android ignore or hide desktop-only ones in generated config and help text.
- Update config loader in `internal/config.go` to generate platform-specific defaults and missing keys from the active schema, not one global desktop schema.

## Test Plan
- Config tests:
  - Android auto-selects `android.conf`.
  - Desktop still uses `curd.conf`.
  - Android config generation excludes desktop-only options.
  - Desktop config generation excludes Android-only options.

- Backend tests:
  - desktop backend still resolves and controls mpv
  - Android intent backend builds correct `am start` command from config
  - Android IPC backend reports unsupported when capability probe fails

- Watch-flow tests:
  - completed episode advances progress and local history in Android IPC mode
  - next episode starts automatically after threshold
  - skip OP/ED seeks are issued through Android IPC backend
  - fallback intent mode disables completion-dependent automation cleanly instead of silently failing

- UX tests:
  - Termux TUI works without alternate screen
  - back/quit/search flows are usable on narrow terminal widths
  - `--android-setup` produces actionable errors when `termux-api` or player integration is missing

## Assumptions
- Android target is Termux on ARM64.
- “All desktop features on Android” is only a valid goal when the chosen Android mpv path exposes MPV-compatible IPC or an equivalent control bridge.
- mpvEx support should be implemented as configurable package/activity targeting, not as a hardcoded special case.
- If no reliable IPC/control path exists for a chosen player, `curd` must downgrade explicitly to basic launch mode and tell the user which features are unavailable.
