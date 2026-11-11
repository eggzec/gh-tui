package config

// Actions configures the files the app downloads.
type Actions struct {
	// DownloadDir is the folder the download picker starts in, and where
	// files go: a leading ~ is the home directory. A missing folder falls
	// back to the home directory.
	DownloadDir string `yaml:"download_dir"`
}
