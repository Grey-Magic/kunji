package cmd

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

const (
	currentVersion = "1.0.3"
	githubRepo     = "Grey-Magic/kunji"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update kunji to the latest version",
	Long:  `Checks GitHub for the latest release and updates the kunji binary if a newer version is available.`,
	Run: func(cmd *cobra.Command, args []string) {
		pterm.Info.Printfln("Current version: %s", currentVersion)
		pterm.Info.Println("Checking for updates...")

		release, err := fetchLatestRelease()
		if err != nil {
			pterm.Error.Printfln("Failed to check for updates: %v", err)
			return
		}

		latestVersion := strings.TrimPrefix(release.TagName, "v")
		if latestVersion == currentVersion {
			pterm.Success.Printfln("You are already on the latest version (%s).", currentVersion)
			return
		}

		pterm.Info.Printfln("New version available: %s → %s", currentVersion, latestVersion)

		assetName := fmt.Sprintf("kunji_%s.zip", latestVersion)
		downloadURL := ""
		for _, asset := range release.Assets {
			if asset.Name == assetName {
				downloadURL = asset.BrowserDownloadURL
				break
			}
		}

		if downloadURL == "" {
			pterm.Error.Printfln("Could not find release asset '%s' for your platform (%s/%s).", assetName, runtime.GOOS, runtime.GOARCH)
			return
		}

		pterm.Info.Printfln("Downloading %s...", assetName)

		zipPath, err := downloadFile(downloadURL)
		if err != nil {
			pterm.Error.Printfln("Download failed: %v", err)
			return
		}
		defer os.Remove(zipPath)

		execPath, err := os.Executable()
		if err != nil {
			pterm.Error.Printfln("Could not determine current executable path: %v", err)
			return
		}
		execPath, err = filepath.EvalSymlinks(execPath)
		if err != nil {
			pterm.Error.Printfln("Could not resolve symlink: %v", err)
			return
		}

		tmpPath := execPath + ".new"
		if err := extractBinaryFromZip(zipPath, tmpPath); err != nil {
			pterm.Error.Printfln("Failed to extract binary: %v", err)
			return
		}

		if err := os.Chmod(tmpPath, 0755); err != nil {
			pterm.Error.Printfln("Failed to set permissions: %v", err)
			os.Remove(tmpPath)
			return
		}

		if err := os.Rename(tmpPath, execPath); err != nil {
			os.Remove(tmpPath)
			if os.IsPermission(err) {
				pterm.Error.Println("Permission denied replacing binary.")
				pterm.Info.Printfln("If kunji is installed in a system directory (e.g. /usr/local/bin), re-run with sudo:")
				pterm.Info.Printfln("  sudo kunji update")
			} else {
				pterm.Error.Printfln("Failed to replace binary: %v", err)
			}
			return
		}

		pterm.Success.Printfln("Updated to version %s successfully!", latestVersion)
	},
}

func fetchLatestRelease() (*githubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "kunji-updater/"+currentVersion)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub response: %v", err)
	}

	return &release, nil
}

func downloadFile(url string) (string, error) {
	tmpFile, err := os.CreateTemp("", "kunji-update-*.zip")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, io.LimitReader(resp.Body, 100<<20)); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

func extractBinaryFromZip(zipPath, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("invalid zip file: %v", err)
	}
	defer r.Close()

	binaryName := "kunji"
	if runtime.GOOS == "windows" {
		binaryName = "kunji.exe"
	}

	for _, f := range r.File {
		if filepath.Base(f.Name) == binaryName {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			out, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer out.Close()

			if _, err := io.Copy(out, io.LimitReader(rc, 100<<20)); err != nil {
				os.Remove(destPath)
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("binary '%s' not found inside the zip archive", binaryName)
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
