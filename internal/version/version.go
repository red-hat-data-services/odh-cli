package version

var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func GetVersion() string {
	return Version
}

func GetCommit() string {
	return Commit
}

func GetDate() string {
	return Date
}