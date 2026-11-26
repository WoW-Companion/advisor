// Command release-prep bundles changes/<ID>.md fragments into one CHANGELOG section and bumps the version.
//
//	release-prep [-bump patch|minor]   write the release (run from the repo root)
//	release-prep -check [-base REF]    fail when a commit edits VERSION or CHANGELOG.md outside a release-prep
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	bump := flag.String("bump", os.Getenv("BUMP"), "patch (default) or minor")
	check := flag.Bool("check", false, "guard: no commit but a release-prep touches VERSION or CHANGELOG.md")
	base := flag.String("base", "origin/main", "ref the branch was cut from (for -check)")
	flag.Parse()
	var err error
	if *check {
		if err = checkFragments("."); err == nil {
			err = checkBranch(".", *base)
		}
	} else {
		var v string
		if v, err = prepare(".", *bump); err == nil {
			fmt.Println("release-prep: prepared", v)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "release-prep:", err)
		os.Exit(1)
	}
}
