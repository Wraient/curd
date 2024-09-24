# # extract the video links from response of embed urls, extract mp4 links form m3u8 lists
get_links() {
    episode_link="$(curl -e "$allanime_refr" -s "https://${allanime_base}$*" -A "$agent" | sed 's|},{|\
|g' | sed -nE 's|.*link":"([^"]*)".*"resolutionStr":"([^"]*)".*|\2 >\1|p;s|.*hls","url":"([^"]*)".*"hardsub_lang":"en-US".*|\1|p')"

    case "$episode_link" in
        *repackager.wixmp.com*)
            extract_link=$(printf "%s" "$episode_link" | cut -d'>' -f2 | sed 's|repackager.wixmp.com/||g;s|\.urlset.*||g')
            for j in $(printf "%s" "$episode_link" | sed -nE 's|.*/,([^/]*),/mp4.*|\1|p' | sed 's|,|\
|g'); do
                printf "%s >%s\n" "$j" "$extract_link" | sed "s|,[^/]*|${j}|g"
            done | sort -nr
            ;;
        *vipanicdn* | *anifastcdn*)
            if printf "%s" "$episode_link" | head -1 | grep -q "original.m3u"; then
                printf "%s" "$episode_link"
            else
                extract_link=$(printf "%s" "$episode_link" | head -1 | cut -d'>' -f2)
                relative_link=$(printf "%s" "$extract_link" | sed 's|[^/]*$||')
                curl -e "$allanime_refr" -s "$extract_link" -A "$agent" | sed 's|^#.*x||g; s|,.*|p|g; /^#/d; $!N; s|\
| >|' | sed "s|>|>${relative_link}|g" | sort -nr
            fi
            ;;
        *) [ -n "$episode_link" ] && printf "%s\n" "$episode_link" ;;
    esac
    # echo $episode_link >> links
    # [ -z "$ANI_CLI_NON_INTERACTIVE" ] && printf "\033[1;32m%s\033[0m Links Fetched\n" "$provider_name" 1>&2
    [ -z "$ANI_CLI_NON_INTERACTIVE" ] 1>&2
}

# # innitialises provider_name and provider_id. First argument is the provider name, 2nd is the regex that matches that provider's link
provider_init() {
    provider_name=$1
    provider_id=$(printf "%s" "$resp" | sed -n "$2" | head -1 | cut -d':' -f2 | sed 's/../&\
/g' | sed 's/^01$/9/g;s/^08$/0/g;s/^05$/=/g;s/^0a$/2/g;s/^0b$/3/g;s/^0c$/4/g;s/^07$/?/g;s/^00$/8/g;s/^5c$/d/g;s/^0f$/7/g;s/^5e$/f/g;s/^17$/\//g;s/^54$/l/g;s/^09$/1/g;s/^48$/p/g;s/^4f$/w/g;s/^0e$/6/g;s/^5b$/c/g;s/^5d$/e/g;s/^0d$/5/g;s/^53$/k/g;s/^1e$/\&/g;s/^5a$/b/g;s/^59$/a/g;s/^4a$/r/g;s/^4c$/t/g;s/^4e$/v/g;s/^57$/o/g;s/^51$/i/g;' | tr -d '\n' | sed "s/\/clock/\/clock\.json/")
}

# # +s links based on given provider
generate_link() {
    case $1 in
        1) provider_init "wixmp" "/Default :/p" ;;     # wixmp(default)(m3u8)(multi) -> (mp4)(multi)
        2) provider_init "dropbox" "/Sak :/p" ;;       # dropbox(mp4)(single)
        3) provider_init "wetransfer" "/Kir :/p" ;;    # wetransfer(mp4)(single)
        4) provider_init "sharepoint" "/S-mp4 :/p" ;;  # sharepoint(mp4)(single)
        *) provider_init "gogoanime" "/Luf-mp4 :/p" ;; # gogoanime(m3u8)(multi)
    esac
    [ -n "$provider_id" ] && get_links "$provider_id"
    # echo $links >> links
}

select_quality() {
    case "$1" in
        best) result=$(printf "%s" "$links" | head -n1) ;;
        worst) result=$(printf "%s" "$links" | grep -E '^[0-9]{3,4}' | tail -n1) ;;
        *) result=$(printf "%s" "$links" | grep -m 1 "$1") ;;
    esac
    [ -z "$result" ] && printf "Specified quality not found, defaulting to best\n" 1>&2 && result=$(printf "%s" "$links" | head -n1)
    printf "%s" "$result" | cut -d'>' -f2
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# # gets embed urls, collects direct links into provider files, selects one with desired quality into $episode
get_episode_url() {
    # get the embed urls of the selected episode
    episode_embed_gql="query (\$showId: String!, \$translationType: VaildTranslationTypeEnumType!, \$episodeString: String!) {    episode(        showId: \$showId        translationType: \$translationType        episodeString: \$episodeString    ) {        episodeString sourceUrls    }}"

    resp=$(curl -e "$allanime_refr" -s -G "${allanime_api}/api" --data-urlencode "variables={\"showId\":\"$id\",\"translationType\":\"$mode\",\"episodeString\":\"$ep_no\"}" --data-urlencode "query=$episode_embed_gql" -A "$agent" | tr '{}' '\n' | sed 's|\\u002F|\/|g;s|\\||g' | sed -nE 's|.*sourceUrl":"--([^"]*)".*sourceName":"([^"]*)".*|\2 :\1|p')
    # generate links into sequential files
    cache_dir="$(mktemp -d)"
    providers="1 2 3 4 5"
    for provider in $providers; do
        generate_link "$provider" >"$cache_dir"/"$provider" &
    done
    wait
    # echo $links > links
    # select the link with matching quality
    links=$(cat "$cache_dir"/* | sed 's|^Mp4-||g;/http/!d' | sort -g -r -s)
    rm -r "$cache_dir"
    episode=$(select_quality "$quality")
    echo $links > /tmp/curd/curd_links
    [ -z "$episode" ] && die "Episode not released!"
}

# setup
agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"
allanime_refr="https://allanime.to"
allanime_base="allanime.day"
allanime_api="https://api.${allanime_base}"
sub_or_dub=$(cat /tmp/curd/curd_mode)
mode="${ANI_CLI_MODE:-$sub_or_dub}"
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
id=$(cat /tmp/curd/curd_id)
ep_no=$(cat /tmp/curd/curd_ep_no)
get_episode_url 
