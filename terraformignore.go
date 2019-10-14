package slug

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/scanner"
)

func parseIgnoreFile(rootPath string) []rule {
	// Look for .terraformignore at our root path/src
	file, err := os.Open(filepath.Join(rootPath, ".terraformignore"))
	defer file.Close()

	// If there's any kind of file error, punt and use the default ignore patterns
	if err != nil {
		// Only show the error debug if an error *other* than IsNotExist
		if !os.IsNotExist(err) {
			fmt.Printf("Error reading .terraformignore, default exclusions will apply: %v \n", err)
		}
		return defaultExclusions
	}
	return readRules(file)
}

func readRules(input io.Reader) []rule {
	rules := defaultExclusions
	scanner := bufio.NewScanner(input)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		pattern := scanner.Text()
		// Ignore blank lines
		if len(pattern) == 0 {
			continue
		}
		// Trim spaces
		pattern = strings.TrimSpace(pattern)
		// Ignore comments
		if pattern[0] == '#' {
			continue
		}
		// New rule structure
		rule := rule{}
		// Exclusions
		if pattern[0] == '!' {
			rule.excluded = true
			pattern = pattern[1:]
		}
		rule.val = pattern
		rule.dirs = strings.Split(pattern, string(os.PathSeparator))
		rules = append(rules, rule)
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading .terraformignore, default exclusions will apply: %v \n", err)
		return defaultExclusions
	}
	return rules
}

func matchIgnoreRule(path string, rules []rule) bool {
	matched := false
	path = filepath.FromSlash(path)
	dir, filename := filepath.Split(path)
	dirSplit := strings.Split(dir, string(os.PathSeparator))

	for _, rule := range rules {
		match, _ := rule.match(path)

		// If no match, try the filename alone
		if !match {
			match, _ = rule.match(filename)
		}

		if !match {
			// Filename check for current directory
			if rule.val[0:1] == "/" && dir == "" {
				rule.val = rule.val[1:]
				rule.compile()
				match, _ = rule.match(filename)
			}
		}

		if !match {
			// Directory checks
			// Does some combination of its parents match our rule?
			for i := 0; i < len(dirSplit); i++ {
				// From the left
				match, _ = rule.match(strings.Join(dirSplit[:i], string(os.PathSeparator)) + string(os.PathSeparator))
				// We found a match! stop whilst ahead
				if match {
					break
				}
			}
		}

		if !match {
			// Directory check for current directory
			// This is a case of say, ignoring terraform.d but NOT ./terraform.d/
			// Since this munges the regex for this pattern, must happen after other directory checks
			if rule.val[0] == '/' {
				rule.val = rule.val[1:]
				rule.compile()
				match, _ = rule.match(dir)
			}
		}

		if match {
			matched = !rule.excluded
		}
	}

	if matched {
		fmt.Printf("Skipping excluded path: %s \n", path)
	}

	return matched
}

type rule struct {
	val      string         // the value of the rule itself
	excluded bool           // ! is present, an exclusion rule
	dirs     []string       // directories of the rule
	regex    *regexp.Regexp // regular expression to match for the rule
}

func (r *rule) match(path string) (bool, error) {
	if r.regex == nil {
		if err := r.compile(); err != nil {
			return false, filepath.ErrBadPattern
		}
	}

	b := r.regex.MatchString(path)
	debug(path, path, r.regex, b)
	return b, nil
}

func (r *rule) compile() error {
	regStr := "^"
	pattern := r.val
	// Go through the pattern and convert it to a regexp.
	// Use a scanner to support utf-8 chars.
	var scan scanner.Scanner
	scan.Init(strings.NewReader(pattern))

	sl := string(os.PathSeparator)
	escSL := sl
	if sl == `\` {
		escSL += `\`
	}

	for scan.Peek() != scanner.EOF {
		ch := scan.Next()
		if ch == '*' {
			if scan.Peek() == '*' {
				// is some flavor of "**"
				scan.Next()

				// Treat **/ as ** so eat the "/"
				if string(scan.Peek()) == sl {
					scan.Next()
				}

				if scan.Peek() == scanner.EOF {
					// is "**EOF" - to align with .gitignore just accept all
					regStr += ".*"
				} else {
					// is "**"
					// Note that this allows for any # of /'s (even 0) because
					// the .* will eat everything, even /'s
					regStr += "(.*" + escSL + ")?"
				}
			} else {
				// is "*" so map it to anything but "/"
				regStr += "[^" + escSL + "]*"
			}
		} else if ch == '?' {
			// "?" is any char except "/"
			regStr += "[^" + escSL + "]"
		} else if ch == '.' || ch == '$' {
			// Escape some regexp special chars that have no meaning
			// in golang's filepath.Match
			regStr += `\` + string(ch)
		} else if ch == '\\' {
			// escape next char. Note that a trailing \ in the pattern
			// will be left alone (but need to escape it)
			if sl == `\` {
				// On windows map "\" to "\\", meaning an escaped backslash,
				// and then just continue because filepath.Match on
				// Windows doesn't allow escaping at all
				regStr += escSL
				continue
			}
			if scan.Peek() != scanner.EOF {
				regStr += `\` + string(scan.Next())
			} else {
				regStr += `\`
			}
		} else {
			regStr += string(ch)
		}
	}

	regStr += "$"
	re, err := regexp.Compile(regStr)
	if err != nil {
		return err
	}

	r.regex = re
	return nil
}

/*
	Default rules:
	.git/
	.terraform/
	!.terraform/modules/
*/

var defaultExclusions = []rule{
	{
		val:      ".git/",
		excluded: false,
	},
	{
		val:      "**/.terraform/",
		excluded: false,
	},
	{
		val:      "**/.terraform/modules/",
		excluded: true,
	},
}

func debug(path string, message ...interface{}) {
	debugPath := os.Getenv("TF_IGNORE_DEBUG")
	if debugPath != "" {
		if strings.Contains(path, debugPath) {
			fmt.Println(message...)
		}
	}
}
