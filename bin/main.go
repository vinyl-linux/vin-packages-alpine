package main

import (
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/schollz/progressbar/v3"
	"github.com/vinyl-linux/go-apk"
)

var (
	// packages holds a map of packages from all
	// APK repositories
	packages = make([]VersionedPackage, 0)

	// providesPackageMap is a mapping of
	// provides -> package, based on the files a
	// package says it provides
	providesPackageMap = make(map[string][]VersionedPackage)

	// errors holds the various errors we've found
	errors = make(map[string]error)

	client = apk.Client{}

	versionSplitRe = regexp.MustCompile(`^(!)?([a-zA-Z0-9.:\/\-\+\_]+)(.*)?$`)
)

type VersionedPackage struct {
	*apk.Package

	VV          *version.Version
	VProvides   []Provides
	VDepends    []Constraint
	DownloadURL string
}

type Constraint struct {
	IsNegate   bool
	Name       string
	Constraint version.Constraints
}

type Provides struct {
	Name    string
	Version *version.Version
}

func init() {
	apk.BaseDir = "."
}

func main() {
	for _, url := range []string{
		apk.BaseURL("v3.15", "main", "x86_64"),
		apk.BaseURL("v3.15", "community", "x86_64"),
	} {
		work(url)
	}

	// now we know what packages and versions we have, we can generate
	// the correct vin config files with the correct version constraints
	bar := progressbar.Default(int64(len(packages)))

	for _, pkg := range packages {
		bar.Add(1)
		deps := make([]*apk.Package, 0)

		for _, dep := range pkg.VDepends {
			d, err := getDep(dep)
			if err != nil {
				errors[pkg.Name] = err

				continue
			}

			deps = append(deps, d.Package)
		}

		// dedupe deps
		deps = dedupe(deps)

		m, err := NewManifest(pkg, deps)
		if err != nil {
			errors[pkg.Name] = err

			continue
		}

		err = m.Write()
		if err != nil {
			errors[pkg.Name] = err
		}
	}

	log.Println("errors:")
	for pkg, err := range errors {
		log.Printf("%q -> %v", pkg, err)
	}
}

func getDep(dep Constraint) (pkg VersionedPackage, err error) {
	if dep.IsNegate {
		return
	}

	pkgs, ok := providesPackageMap[dep.Name]
	if !ok {
		err = fmt.Errorf("unknown dependency %q", dep.Name)

		return
	}

	if len(pkgs) == 1 {
		return pkgs[0], nil
	}

	sort.SliceStable(pkgs, func(i, j int) bool {
		return pkgs[i].Version > pkgs[j].Version
	})

	for _, pkg := range pkgs {
		if dep.Constraint.Check(pkg.VV) {
			return pkg, nil
		}

		for _, provides := range pkg.VProvides {
			if provides.Version == nil {
				log.Printf("dep %s provides %s has a nil version", pkg.Name, dep.Name)

				continue
			}

			if dep.Constraint.Check(provides.Version) {
				return pkg, nil
			}
		}
	}

	err = fmt.Errorf("could not match %v", dep)

	return
}

func depSplit(s string) (c Constraint, err error) {
	if s == "" {
		return
	}

	elems := versionSplitRe.FindAllStringSubmatch(s, -1)
	if len(elems) != 1 {
		err = fmt.Errorf("uhh.... somehow %q returned %d matches", s, len(elems))

		return
	}

	match := elems[0]
	if len(match) != 4 {
		err = fmt.Errorf("uhh.... somehow %v has %d groups", match, len(match))

		return
	}

	c.IsNegate = match[1] == "!"
	c.Name = match[2]

	if match[3] != "" {
		if match[3][0] == '~' {
			match[3] = "=" + match[3][1:]
		}

		c.Constraint, err = version.NewConstraint(cleanVersion(string(match[3])))
	}

	return
}

func providesSplit(s string) (p Provides, err error) {
	elems := strings.Split(s, "=")
	switch len(elems) {
	case 0:

	case 1:
		p.Name = s
	case 2:
		p.Name = elems[0]
		p.Version, err = version.NewVersion(cleanVersion(elems[1]))

	default:
		err = fmt.Errorf("unable to process provides string %q", s)
	}

	return
}

func cleanVersion(s string) string {
	ss := strings.Split(s, "_")

	return ss[0]
}

func NewVersionedPackage(baseurl string, pkg *apk.Package) (vp VersionedPackage, err error) {
	vp.DownloadURL = baseurl
	vp.VV, err = version.NewVersion(cleanVersion(pkg.Version))
	if err != nil {
		return
	}

	vp.Package = pkg
	vp.VDepends = make([]Constraint, len(pkg.Dependencies))
	vp.VProvides = make([]Provides, len(pkg.Provides))

	for idx, d := range pkg.Dependencies {
		vp.VDepends[idx], err = depSplit(d)
		if err != nil {
			return
		}
	}

	for idx, p := range pkg.Provides {
		vp.VProvides[idx], err = providesSplit(p)
		if err != nil {
			return
		}
	}

	return
}

func work(url string) {
	client.BaseURL = url

	fn, err := client.DownloadIndex()
	if err != nil {
		panic(err)
	}

	// parse index
	a, err := apk.NewAPKIndex(fn)
	if err != nil {
		panic(err)
	}

	for _, pkg := range a.Packages {
		if pkg.Name == "" {
			continue
		}

		versionPkg, err := NewVersionedPackage(url, pkg)
		if err != nil {
			log.Printf("Skipping %s with error %v", pkg.Name, err)

			continue
		}

		packages = append(packages, versionPkg)

		if _, ok := providesPackageMap[pkg.Name]; !ok {
			providesPackageMap[pkg.Name] = make([]VersionedPackage, 0)
		}

		providesPackageMap[pkg.Name] = append(providesPackageMap[pkg.Name], versionPkg)

		for _, provides := range versionPkg.VProvides {
			if _, ok := providesPackageMap[provides.Name]; !ok {
				providesPackageMap[provides.Name] = make([]VersionedPackage, 0)
			}

			providesPackageMap[provides.Name] = append(providesPackageMap[provides.Name], versionPkg)
		}
	}
}

func dedupe(deps []*apk.Package) (out []*apk.Package) {
	out = make([]*apk.Package, 0)
	for _, dep := range deps {
		if dep == nil || dep.Name == "" {
			continue
		}

		if !contains(out, dep.Name) {
			out = append(out, dep)
		}
	}

	return
}

func contains(deps []*apk.Package, name string) bool {
	for _, dep := range deps {
		if dep.Name == name {
			return true
		}
	}

	return false
}
