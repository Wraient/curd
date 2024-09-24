search_anime() {
    search_gql="query(        \$search: SearchInput        \$limit: Int        \$page: Int        \$translationType: VaildTranslationTypeEnumType        \$countryOrigin: VaildCountryOriginEnumType    ) {    shows(        search: \$search        limit: \$limit        page: \$page        translationType: \$translationType        countryOrigin: \$countryOrigin    ) {        edges {            _id name availableEpisodes __typename       }    }}"

    curl -e "$allanime_refr" -s -G "${allanime_api}/api" --data-urlencode "variables={\"search\":{\"allowAdult\":false,\"allowUnknown\":false,\"query\":\"$1\"},\"limit\":40,\"page\":1,\"translationType\":\"$mode\",\"countryOrigin\":\"ALL\"}" --data-urlencode "query=$search_gql" -A "$agent" | sed 's|Show|\
|g' | sed -nE "s|.*_id\":\"([^\"]*)\",\"name\":\"([^\"]*)\".*${mode}\":([1-9][^,]*).*|\1	\2 (\3 episodes)NEWLINEFROMHERE|p"
}

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

# Read the entire file content into a variable
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
query=$(cat /tmp/curd/curd_query)  # This is a shorthand for cat "$filename"

# query="one piece"
anime_list=$(search_anime "$query")

echo $anime_list > /tmp/curd/curd_anime_list
