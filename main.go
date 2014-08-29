package main

import (
	"flag"
	"fmt"
	"go/build"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var (
	dry               bool
	rewrites          map[string]string // rewrites that have been performed
	visited           map[string]bool   // packages that have already been visited
	gopath            string            // the last component of GOPATH
	verbose           bool              // flag to indicate verbose output
	forceUpdates      bool              // flag to force updating packages already vendorized
	updateImports     bool              // flag to specify that imports should be updated in files
	packagesRemaining int               // total number of packages remaining. used to track goroutines still in progress.
)

// stringSliceFlag is a flag.Value that accumulates multiple flags in to a slice.
type stringSliceFlag []string

type vendorizeResult struct {
	path string
	err  error
}

// formats the stringSliceFlag
func (s *stringSliceFlag) String() string {
	return fmt.Sprintf("%v", []string(*s))
}

func (s *stringSliceFlag) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// package prefixes that should not be copied
var blacklistedPrefixes stringSliceFlag

// builtPackages maintains a cache of package builds.
var builtPackages map[string]*build.Package

func main() {

	start := time.Now()

	flag.BoolVar(&dry, "d", false, "If true, perform a dry run but don't execute anything.")
	flag.BoolVar(&verbose, "v", false, "Provide verbose output")
	flag.Var(&blacklistedPrefixes, "b", "Package prefix to blacklist. Can be given multiple times.")
	flag.BoolVar(&forceUpdates, "f", false, "If true, forces updates on already vendorized packages.")
	flag.BoolVar(&updateImports, "u", false, "If true, updates import statements for vendorized packages.")
	flag.Parse()

	// set the go path
	gopaths := filepath.SplitList(os.Getenv("GOPATH"))
	gopath = gopaths[len(gopaths)-1]
	if gopath == "" {
		log.Fatal("GOPATH must be set")
	}

	// set the package name from arguments
	pkgName := flag.Arg(0)
	if pkgName == "" {
		log.Fatal("Package name required")
	}

	// set the destination from arguments
	dest := flag.Arg(1)
	if dest == "" {
		log.Fatal("Destination path required")
	}

	blacklistedPrefixes = append(blacklistedPrefixes, pkgName)
	blacklistedPrefixes = append(blacklistedPrefixes, dest)
	rewrites = make(map[string]string)
	visited = make(map[string]bool)

	ch := make(chan vendorizeResult)

	packagesRemaining++
	go vendorize(pkgName, dest, ch)

	for packagesRemaining > 0 {
		select {
		case r := <-ch:

			visited[r.path] = true
			packagesRemaining--

			if r.err != nil {
				verbosef("[Packages Remaining: %d] %s\n", packagesRemaining, r.err.Error())
			} else {
				verbosef("[Packages Remaining: %d] Package vendorized %s\n", packagesRemaining, r.path)
			}
		}
	}

	log.Printf("Vendorized %d imports in %v", len(rewrites), time.Since(start))
}

// vendorize the package located at path, placing copied files in dest
func vendorize(path, dest string, ch chan vendorizeResult) {

	verbosef("Vendorizing %s", path)

	result := vendorizeResult{path: path, err: nil}

	if visited[path] {
		result.err = fmt.Errorf("Path '%v' already visited... skipping", path)
		ch <- result
		return
	}

	// build the package
	rootPkg, err := buildPackage(path)
	if err != nil {
		result.err = fmt.Errorf("Couldn't import %s: %s", path, err)
		ch <- result
		return
	}
	if rootPkg.Goroot {
		result.err = fmt.Errorf("Can't vendorize packages from GOROOT")
		ch <- result
		return
	}

	// get import statements
	allImports := getAllImports(rootPkg)

	var pkgs []*build.Package
	for _, imp := range allImports {
		if imp == "C" {
			continue
		}
		pkg, err := buildPackage(imp)
		if err != nil {
			result.err = fmt.Errorf("%s: couldn't import %s: %s", path, imp, err)
			ch <- result
			return
		}
		if !pkg.Goroot {
			pkgs = append(pkgs, pkg)
		}
	}

	// Recursively vendorize imports
	for _, pkg := range pkgs {
		if pkg.ImportPath == path {
			// Don't recurse into self.
			continue
		}
		if !visited[pkg.ImportPath] {
			packagesRemaining++
			go vendorize(pkg.ImportPath, dest, ch)
		}
	}

	pkgDir := rootPkg.Dir

	// only copy packages when they aren't ignored
	if !ignored(path) {
		newPath := dest + "/" + path
		pkgDir = filepath.Join(gopath, "src", newPath)
		// only overwrite files if specifically requested to do so
		fileExists, _ := exists(pkgDir)
		if forceUpdates || !fileExists {
			err = copyDir(pkgDir, rootPkg.Dir)
			if err != nil {
				result.err = fmt.Errorf("Couldn't copy %s: %s", path, err)
				ch <- result
				return
			}
			rewrites[path] = newPath
		} else {
			result.err = fmt.Errorf("Ignored (preexisting): %q", pkgDir)
			ch <- result
			return
		}
	}

	// Rewrite any import lines in the package, but only on request
	if updateImports {
		for _, files := range [][]string{
			rootPkg.GoFiles, rootPkg.CgoFiles, rootPkg.TestGoFiles, rootPkg.XTestGoFiles,
		} {
			for _, file := range files {
				if len(rewrites) > 0 {
					destFile := filepath.Join(pkgDir, file)
					verbosef("Rewriting imports in %q", destFile)
					err := rewriteFile(destFile, filepath.Join(rootPkg.Dir, file), rewrites)
					if err != nil {
						result.err = fmt.Errorf("%s: couldn't rewrite file %q: %s", path, file, err)
						ch <- result
						return
					}
				}
			}
		}
	}

	ch <- result
	return
}

// checks for the existence of the file located at filepath
func exists(filepath string) (bool, error) {
	_, err := os.Stat(filepath)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// determines if the path contains an ignored prefix
func ignored(path string) bool {
	_, rewritten := rewrites[path]
	if rewritten {
		return true
	}
	for _, prefix := range blacklistedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// copyFile copies the file given by src to dest, creating dest with the permissions given by perm.
func copyFile(dest, src string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)

	return err
}

// copyDir non-recursively copies the contents of the src directory to dest.
func copyDir(dest, src string) error {
	verbosef("Copying contents of %q to %q", src, dest)
	if !dry {
		err := os.MkdirAll(dest, 0770)
		if err != nil {
			return fmt.Errorf("Couldn't make destination directory %v", dest)
		}
	}

	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// We don't recurse.
		if info.IsDir() {
			if path != src {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		destFile := filepath.Join(dest, relPath)
		verbosef("Copying %q to %q", path, destFile)
		if dry {
			return nil
		}

		doesExist, err := exists(destFile)

		if err != nil {
			return err
		}

		if !doesExist || forceUpdates {
			copyFile(destFile, path, info.Mode().Perm())
		}

		return nil
	})
}

// returns a list of all import paths in the Go files of pkg.
func getAllImports(pkg *build.Package) []string {
	allImports := make(map[string]bool)
	for _, imports := range [][]string{pkg.Imports, pkg.TestImports, pkg.XTestImports} {
		for _, imp := range imports {
			allImports[imp] = true
		}
	}
	result := make([]string, 0, len(allImports))
	for imp := range allImports {
		result = append(result, imp)
	}
	return result
}

// buildPackage builds a package given by the path.
func buildPackage(path string) (*build.Package, error) {
	if builtPackages == nil {
		builtPackages = make(map[string]*build.Package)
	}
	if pkg, ok := builtPackages[path]; ok {
		return pkg, nil
	}

	ctx := build.Default

	pkg, err := ctx.Import(path, "", 0)
	if err != nil {
		return nil, err
	}
	builtPackages[path] = pkg
	return pkg, nil
}

// rewrites the file at path with new import statements
func rewriteFile(dest, path string, m map[string]string) error {
	if dry {
		return nil
	}

	f, err := ioutil.TempFile("", "vendorize")
	if err != nil {
		return err
	}
	defer f.Close()
	err = rewriteFileImports(path, m, f)
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), dest)
}

// rewrites the file import statements to the new location
func rewriteFileImports(path string, m map[string]string, w io.Writer) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return err
	}

	for _, s := range f.Imports {
		path, err := strconv.Unquote(s.Path.Value)
		if err != nil {
			panic(err)
		}
		if replacement, ok := m[path]; ok {
			s.Path.Value = strconv.Quote(replacement)
		}
	}

	return printer.Fprint(w, fset, f)
}

// verbosef logs only if verbose is true.
func verbosef(s string, args ...interface{}) {
	if verbose {
		log.Printf(s, args...)
	}
}
