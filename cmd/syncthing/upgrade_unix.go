// +build !windows

package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"bitbucket.org/kardianos/osext"
)

type githubRelease struct {
	Tag      string        `json:"tag_name"`
	Prelease bool          `json:"prerelease"`
	Assets   []githubAsset `json:"assets"`
}

type githubAsset struct {
	URL  string `json:"url"`
	Name string `json:"name"`
}

var GoArchExtra string // "", "v5", "v6", "v7"

func upgrade() error {
	path, err := osext.Executable()
	if err != nil {
		return err
	}

	resp, err := http.Get("https://api.github.com/repos/calmh/syncthing/releases?per_page=1")
	if err != nil {
		return err
	}

	var rels []githubRelease
	json.NewDecoder(resp.Body).Decode(&rels)
	resp.Body.Close()

	if len(rels) != 1 {
		return fmt.Errorf("Unexpected number of releases: %d", len(rels))
	}
	rel := rels[0]

	if rel.Tag > Version {
		l.Infof("Attempting upgrade to %s...", rel.Tag)
	} else if rel.Tag == Version {
		l.Okf("Already running the latest version, %s. Not upgrading.", Version)
		return nil
	} else {
		l.Okf("Current version %s is newer than latest release %s. Not upgrading.", Version, rel.Tag)
		return nil
	}

	expectedRelease := fmt.Sprintf("syncthing-%s-%s%s-%s.", runtime.GOOS, runtime.GOARCH, GoArchExtra, rel.Tag)
	for _, asset := range rel.Assets {
		if strings.HasPrefix(asset.Name, expectedRelease) {
			if strings.HasSuffix(asset.Name, ".tar.gz") {
				l.Infof("Downloading %s...", asset.Name)
				fname, err := readTarGZ(asset.URL, filepath.Dir(path))
				if err != nil {
					return err
				}

				old := path + "." + Version
				err = os.Rename(path, old)
				if err != nil {
					return err
				}
				err = os.Rename(fname, path)
				if err != nil {
					return err
				}

				l.Okf("Upgraded %q to %s.", path, rel.Tag)
				l.Okf("Previous version saved in %q.", old)

				return nil
			}
		}
	}

	l.Warnf("Found no asset for %q", expectedRelease)
	return nil
}

func readTarGZ(url string, dir string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("Accept", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	gr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", err
	}

	tr := tar.NewReader(gr)
	if err != nil {
		return "", err
	}

	// Iterate through the files in the archive.
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			return "", err
		}

		if path.Base(hdr.Name) == "syncthing" {
			of, err := ioutil.TempFile(dir, "syncthing")
			if err != nil {
				return "", err
			}
			io.Copy(of, tr)
			err = of.Close()
			if err != nil {
				os.Remove(of.Name())
				return "", err
			}

			os.Chmod(of.Name(), os.FileMode(hdr.Mode))
			return of.Name(), nil
		}
	}

	return "", fmt.Errorf("No upgrade found")
}
