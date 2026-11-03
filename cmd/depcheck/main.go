// Command depcheck verifies the integrity of bundled and downloaded external
// components against the dependency catalog. It is used by CI so a release can
// never ship a wrong or tampered binary.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"streamdrop/internal/deps"
)

func main() {
	staging := flag.String("staging", "", "directory with downloaded release archives")
	depsDir := flag.String("deps", "", "directory with the portable deps/<id>/<version> layout")
	goos := flag.String("os", runtime.GOOS, "target platform to verify")
	goarch := flag.String("arch", runtime.GOARCH, "target architecture to verify")
	allowUnpinned := flag.Bool("allow-unpinned", false, "warn instead of failing when a hash is not pinned")
	flag.Parse()

	fmt.Printf("depcheck: %s/%s, catalog %s\n", *goos, *goarch, deps.CatalogVersionForLog())

	failed := false
	for _, c := range deps.Catalog {
		archiveHash := c.ArchiveHashFor(*goos, *goarch)
		binaryHash := c.BinaryHashFor(*goos, *goarch)

		// Archive verification (skipped for single-file components).
		if *staging != "" && c.ArchiveName != "" {
			name := deps.ArchiveFileName(c, *goos, *goarch)
			path := filepath.Join(*staging, name)
			if _, err := os.Stat(path); err != nil {
				// CI stores archives under their own names; try an exact name match.
				if found := findByName(*staging, name); found != "" {
					path = found
				} else {
					fmt.Printf("MISSING archive %s (%s)\n", c.ID, path)
					failed = true
					continue
				}
			}
			sum, ok := deps.VerifyArchiveFor(path, c, *goos, *goarch)
			switch {
			case archiveHash == "" && !*allowUnpinned:
				fmt.Printf("UNPINNED %s archive %s (sum %s)\n", c.ID, name, sum)
				failed = true
			case !ok && archiveHash != "":
				fmt.Printf("BAD %s archive: got %s want %s\n", c.ID, sum, archiveHash)
				failed = true
			default:
				fmt.Printf("OK %s archive %s\n", c.ID, short(sum))
			}
		}

		if *depsDir == "" {
			continue
		}
		dir := filepath.Join(*depsDir, c.ID, c.Version)
		bin := deps.BinaryPath(dir, c)
		if _, err := os.Stat(bin); err != nil {
			fmt.Printf("MISSING binary %s (%s)\n", c.ID, bin)
			failed = true
			continue
		}
		sum, ok := deps.VerifyBinaryFor(bin, c, *goos, *goarch)
		switch {
		case binaryHash == "" && !*allowUnpinned:
			fmt.Printf("UNPINNED %s binary (sum %s)\n", c.ID, sum)
			failed = true
		case binaryHash == "":
			fmt.Printf("SKIP %s binary (no pinned hash for %s/%s)\n", c.ID, *goos, *goarch)
		case !ok:
			fmt.Printf("BAD %s binary: got %s want %s\n", c.ID, sum, binaryHash)
			failed = true
		default:
			fmt.Printf("OK %s binary %s\n", c.ID, short(sum))
		}
	}

	if failed {
		fmt.Println("depcheck: FAILED")
		os.Exit(1)
	}
	fmt.Println("depcheck: OK")
}

func findByName(dir, name string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Base(e.Name()) == name {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12] + "…"
	}
	return s
}
