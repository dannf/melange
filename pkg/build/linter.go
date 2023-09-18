// Copyright 2023 Chainguard, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package build

import (
	"fmt"
	"io/fs"
	"regexp"

	"chainguard.dev/melange/pkg/config"
)

type LinterContext struct {
	pkgname string
	cfg     *config.Configuration
	chk     *config.Checks
}

type LinterFunc func(lctx LinterContext, path string, d fs.DirEntry) error

type Linter struct {
	LinterFunc LinterFunc
	Explain    string
}

var Linters = map[string]Linter{
	"setuidgid": Linter{isSetUidOrGidLinter, "Unset the setuid/setgid bit on the relevant files, or remove this linter"},
	"tempdir":   Linter{tempDirLinter, "Remove any offending files in temporary dirs in the pipeline"},
	"usrlocal":  Linter{usrLocalLinter, "This package should be a -compat package"},
	"varempty":  Linter{varEmptyLinter, "Remove any offending files in /var/empty in the pipeline"},
}

var isUsrLocalRegex = regexp.MustCompile("^usr/local/")
var isVarEmptyRegex = regexp.MustCompile("^var/empty/")
var isTempDirRegex = regexp.MustCompile("^(var/)?(tmp|run)/")
var isCompatPackage = regexp.MustCompile("-compat$")

func usrLocalLinter(lctx LinterContext, path string, _ fs.DirEntry) error {
	// If this is already a compat package, do nothing.
	if isCompatPackage.MatchString(lctx.pkgname) {
		return nil
	}

	if isUsrLocalRegex.MatchString(path) {
		return fmt.Errorf("/usr/local path found in non-compat package")
	}

	return nil
}

func varEmptyLinter(lctx LinterContext, path string, _ fs.DirEntry) error {
	if isVarEmptyRegex.MatchString(path) {
		return fmt.Errorf("Package writes to /var/empty")
	}

	return nil
}

func tempDirLinter(lctx LinterContext, path string, _ fs.DirEntry) error {
	if isTempDirRegex.MatchString(path) {
		return fmt.Errorf("Package writes to a temp dir")
	}

	return nil
}

func isSetUidOrGidLinter(lctx LinterContext, path string, d fs.DirEntry) error {
	info, err := d.Info()
	if err != nil {
		return err
	}

	mode := info.Mode()
	if mode&fs.ModeSetuid != 0 {
		return fmt.Errorf("File is setuid")
	} else if mode&fs.ModeSetgid != 0 {
		return fmt.Errorf("File is setgid")
	}

	return nil
}

func lintPackageFs(lctx LinterContext, fsys fs.FS, linters []string) error {
	walkCb := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("Error traversing tree at %s: %w", path, err)
		}

		for _, linterName := range linters {
			linter, present := Linters[linterName]
			if !present {
				return fmt.Errorf("Linter %s is unknown", linterName)
			}

			err = linter.LinterFunc(lctx, path, d)
			if err != nil {
				return fmt.Errorf("Linter %s failed at path \"%s\": %w; suggest: %s", linterName, path, err, linter.Explain)
			}
		}

		return nil
	}

	err := fs.WalkDir(fsys, ".", walkCb)
	if err != nil {
		return err
	}

	return nil
}
