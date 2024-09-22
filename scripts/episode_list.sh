episodes_list() {
    episodes_list_gql="query (\$showId: String!) {    show(        _id: \$showId    ) {        _id availableEpisodesDetail    }}"

    curl -e "$allanime_refr" -s -G "${allanime_api}/api" --data-urlencode "variables={\"showId\":\"$*\"}" --data-urlencode "query=$episodes_list_gql" -A "$agent" | sed -nE "s|.*$mode\":\[([0-9.\",]*)\].*|\1|p" | sed 's|,|\
|g; s|"||g' | sort -n -k 1
}

# setup
agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"
allanime_refr="https://allanime.to"
allanime_base="allanime.day"
allanime_api="https://api.${allanime_base}"
mode="${ANI_CLI_MODE:-sub}"
download_dir="${ANI_CLI_DOWNLOAD_DIR:-.}"
quality="${ANI_CLI_QUALITY:-best}"
case "$(uname -a)" in
    *Darwin*) player_function="${ANI_CLI_PLAYER:-iina}" ;;            # mac OS
    *ndroid*) player_function="${ANI_CLI_PLAYER:-android_mpv}" ;;     # Android OS (termux)
    *steamdeck*) player_function="${ANI_CLI_PLAYER:-flatpak_mpv}" ;;  # steamdeck OS
    *MINGW* | *WSL2*) player_function="${ANI_CLI_PLAYER:-mpv.exe}" ;; # Windows OS
    *ish*) player_function="${ANI_CLI_PLAYER:-iSH}" ;;                # iOS (iSH)
    *) player_function="${ANI_CLI_PLAYER:-mpv}" ;;                    # Linux OS
esac
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
id=$(cat $script_dir/tmp/id)
echo $(episodes_list "$id") > $script_dir/tmp/episode_list
