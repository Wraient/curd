package internal

var (
	globalAnime   *Anime
	globalLogFile string
)

// SetGlobalAnime sets the global anime reference
func SetGlobalAnime(anime *Anime) {
	globalAnime = anime
}

// GetGlobalAnime gets the global anime reference
func GetGlobalAnime() *Anime {
	return globalAnime
}

// SetGlobalLogFile sets the global log file path
func SetGlobalLogFile(logFile string) {
	globalLogFile = logFile
}

// GetGlobalLogFile gets the global log file path
func GetGlobalLogFile() string {
	return globalLogFile
}
