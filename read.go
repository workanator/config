// Copyright 2009  The "config" Authors
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

package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var (
	// The regexp test if the path contains globs * and/or ?
	reGlob = regexp.MustCompile(`[\*\?]`)
	// The regexp is for matching include file directive
	reIncludeFile = regexp.MustCompile(`^#include\s+(.+?)\s*$`)
)

// configFile identifies a file which should be read.
type configFile struct {
	Path string
	Read bool
}

// fileList is the list of files to read.
type fileList []*configFile

// pushFile converts the path into the absolute path and pushes the file
// into the list if it does not contain the same absolute path already.
// All relative paths are relative to the main file which is the first
// file in the list.
func (list *fileList) pushFile(path string) error {
	var (
		absPath string
		err     error
	)

	// Convert the path into the absolute path
	if !filepath.IsAbs(path) {
		// Make the path relative to the main file
		var relPath string
		if len(*list) > 0 {
			// Join the relative path with the main file path
			relPath = filepath.Join(filepath.Dir((*list)[0].Path), path)
		} else {
			relPath = path
		}

		if absPath, err = filepath.Abs(relPath); err != nil {
			return err
		}
	} else {
		absPath = path
	}

	// Make the list of file candidates to include
	var candidates []string
	if reGlob.MatchString(absPath) {
		candidates, err = filepath.Glob(absPath)
		if err != nil {
			return err
		}
	} else {
		candidates = []string{absPath}
	}

	for _, candidate := range candidates {
		// Test the file with the absolute path exists in the list
		for _, file := range *list {
			if file.Path == candidate {
				return nil
			}
		}

		// Push the new file to the list
		*list = append(*list, &configFile{
			Path: candidate,
			Read: false,
		})
	}

	return nil
}

// _read reads file list
func _read(c *Config, list *fileList) (*Config, error) {
	// Pass through the list untill all files are read
	for {
		hasUnread := false

		// Go through the list and read files
		for _, file := range *list {
			if !file.Read {
				if err := _readFile(file.Path, c, list); err != nil {
					return nil, err
				}

				file.Read = true
				hasUnread = true
			}
		}

		// Exit the loop because all files are read
		if !hasUnread {
			break
		}
	}

	return c, nil
}

// _readFile is the base to read a file and get the configuration representation.
// That representation can be queried with GetString, etc.
func _readFile(fname string, c *Config, list *fileList) error {
	file, err := os.Open(fname)
	if err != nil {
		return err
	}

	// Defer closing the file so we can be sure the underlying file handle
	// will be closed in any case.
	defer file.Close()

	if err = c.read(bufio.NewReader(file), list); err != nil {
		return err
	}

	if err = file.Close(); err != nil {
		return err
	}

	return nil
}

// Read reads a configuration file and returns its representation.
// All arguments, except `fname`, are related to `New()`
func Read(fname string, comment, separator string, preSpace, postSpace bool) (*Config, error) {
	list := &fileList{}
	list.pushFile(fname)

	return _read(New(comment, separator, preSpace, postSpace), list)
}

// ReadDefault reads a configuration file and returns its representation.
// It uses values by default.
func ReadDefault(fname string) (*Config, error) {
	list := &fileList{}
	list.pushFile(fname)

	return _read(NewDefault(), list)
}

// * * *

func (c *Config) read(buf *bufio.Reader, list *fileList) (err error) {
	var section, option string
	var scanner = bufio.NewScanner(buf)
	for scanner.Scan() {
		l := strings.TrimRightFunc(stripComments(scanner.Text()), unicode.IsSpace)

		// Switch written for readability (not performance)
		switch {
		// Empty line and comments
		case len(l) == 0, l[0] == ';':
			continue

		// Comments starting with ;
		case l[0] == '#':
			// Test for possible directives
			if matches := reIncludeFile.FindStringSubmatch(l); matches != nil {
				list.pushFile(matches[1])
			} else {
				continue
			}

		// New section. The [ must be at the start of the line
		case l[0] == '[' && l[len(l)-1] == ']':
			option = "" // reset multi-line value
			section = strings.TrimSpace(l[1 : len(l)-1])
			c.AddSection(section)

		// Continuation of multi-line value
		// starts with whitespace, we're in a section and working on an option
		case section != "" && option != "" && (l[0] == ' ' || l[0] == '\t'):
			prev, _ := c.RawString(section, option)
			value := strings.TrimSpace(l)
			c.AddOption(section, option, prev+"\n"+value)

		// Other alternatives
		default:
			i := strings.IndexAny(l, "=:")

			switch {
			// Option and value
			case i > 0 && l[0] != ' ' && l[0] != '\t': // found an =: and it's not a multiline continuation
				option = strings.TrimSpace(l[0:i])
				value := strings.TrimSpace(l[i+1:])
				c.AddOption(section, option, value)

			default:
				return errors.New("could not parse line: " + l)
			}
		}
	}
	return scanner.Err()
}
