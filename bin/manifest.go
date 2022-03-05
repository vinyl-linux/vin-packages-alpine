package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hashicorp/go-version"
	"github.com/pelletier/go-toml"
	"github.com/vinyl-linux/go-apk"
	"github.com/zeebo/blake3"
)

var (
	skipCommand    = "echo"
	installCommand = "{{ .ManifestDir }}/install.sh"
)

// Dep represents a dependency tuple.
//
// A dependency tuple is characterised as (package, version constraint)
type Dep [2]string

// Manifest represents the config needed to install a package, including
// build commands, dependencies, sources, versions, and associated funtions
// that do stuff with those things
type Manifest struct {
	ID string `toml:"-"`

	Provides   string           `toml:"provides"`
	VersionStr string           `toml:"version"`
	Version    *version.Version `toml:"-"`
	Checksum   string           `toml:"checksum"`
	Licence    string           `toml:"licence"`
	Tarball    string           `toml:"tarball"`
	Meta       bool             `toml:"meta,omitempty"`
	ServiceDir string           `toml:"service_directory,omitempty"`

	Profiles map[string]Profile `toml:"profiles"`
	Commands Commands           `toml:"commands"`

	dir      string
	filename string
}

func NewManifest(vp VersionedPackage, deps []VersionedPackage) (m Manifest, err error) {
	m.Provides = vp.Name
	m.VersionStr = vp.VV.String()
	m.Licence = vp.Licence

	m.Commands.WorkingDir = "."
	m.Commands.Configure = &skipCommand
	m.Commands.Compile = &skipCommand
	m.Commands.Install = &installCommand

	m.dir = filepath.Join(apk.BaseDir, m.Provides, m.VersionStr)
	m.filename = filepath.Join(m.dir, "manifest.toml")

	// if file exists, skip
	_, err = os.Stat(m.filename)
	if err == nil || !os.IsNotExist(err) {
		return
	}

	m.Profiles = make(map[string]Profile)
	m.Profiles["default"] = Profile{
		Deps: make([]Dep, len(deps)),
	}

	for idx, dep := range deps {
		m.Profiles["default"].Deps[idx] = [2]string{dep.Name, dep.VV.String()}
	}

	client.BaseURL = vp.baseurl
	m.Tarball = fmt.Sprintf("%s/%s-%s.apk", client.BaseURL, vp.Name, vp.Version)

	fn, err := client.DownloadPackage(vp.Package)
	if err != nil {
		return
	}

	m.Checksum, err = checksum(fn)
	if err != nil {
		return
	}

	err = os.Remove(fn)

	return
}

func (m Manifest) Write() (err error) {
	err = os.MkdirAll(m.dir, 0750)
	if err != nil {
		return
	}

	f, err := os.Create(m.filename)
	if err != nil {
		return
	}

	defer f.Close()

	b, err := toml.Marshal(m)
	if err != nil {
		return
	}

	_, err = f.Write(b)
	if err != nil {
		return
	}

	symlink := filepath.Join(m.dir, "install.sh")
	_, err = os.Stat(symlink)
	if err != nil && os.IsNotExist(err) {
		return os.Symlink("../../scripts/install.sh", symlink)
	}

	return
}

func checksum(fn string) (sum string, err error) {
	f, err := os.Open(fn)
	if err != nil {
		return
	}

	defer f.Close()

	h := blake3.New()
	_, err = io.Copy(h, f)
	if err != nil {
		return
	}

	outB := make([]byte, 0)
	sumB := h.Sum(outB)

	sum = hex.EncodeToString(sumB)

	return
}

// Profile holds a set of dependencies associated with a 'profile'.
//
// A 'profile' is a way of splitting dependencies into groups, such
// as only including GUI dependencies when building X11 apps, or
// bundling extra packages for larger/ less disk constrained systems
type Profile struct {
	Deps []Dep `toml:"deps"`
}

// Commands provides a set of 'commands' which are used in our three stages:
//
//   1. Configuring packages/ apps/ whosits
//   2. Compiling packages/ binaries
//   3. Installing the resulting compiled stuff into the filesystem
//
// Empty commands receive the default for each item, so use something like `true`
// where a stage command is not necessary
//
// They also include an optional 'WorkingDir' which is appended to manifest.dir
type Commands struct {
	Configure  *string  `toml:"configure,omitempty"`
	Compile    *string  `toml:"compile,omitempyy"`
	Install    *string  `toml:"install,omitempty"`
	WorkingDir string   `toml:"working_dir,omitempty"`
	Patches    []string `toml:"patches,omitempty"`
	Skipenv    bool     `toml:"skipenv,omitempty"`
}
