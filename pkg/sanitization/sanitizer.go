package sanitization

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const randomSuffixOffset = 8 // accounts for random suffix generated by Pulumi

type (
	Sanitizer struct {
		rules     []Rule
		maxLength int
	}

	Rule struct {
		Pattern     *regexp.Regexp
		Replacement string
		// Lowercase represents a rule which is meant to just run toLowerCase on the string
		Lowercase bool
		// Uppercase represents a rule which is meant to just run toUpperCase on the string
		Uppercase bool
	}
)

// Apply sequentially applies a Sanitizer's rules to the supplied input and returns the sanitized result.
func (s *Sanitizer) Apply(input string) string {
	maxLength := s.maxLength - randomSuffixOffset
	output := input
	for _, rule := range s.rules {
		output = rule.Pattern.ReplaceAllString(output, rule.Replacement)
		if rule.Lowercase {
			output = strings.ToLower(output)
		}
		if rule.Uppercase {
			output = strings.ToUpper(output)
		}
	}
	if maxLength > 0 && maxLength < len(output) {
		overflow := output[s.maxLength-8:]
		h := sha256.New()
		if json.NewEncoder(h).Encode(overflow) != nil {
			return output
		}
		hash := fmt.Sprintf("%x", h.Sum(nil))
		output = fmt.Sprintf("%s%s", output[0:maxLength-8], hash[0:8])
	}
	return output
}

// NewSanitizer returns a new Sanitizer that applies the supplied rules to inputs.
func NewSanitizer(rules []Rule, maxLength int) *Sanitizer {
	return &Sanitizer{rules: rules, maxLength: maxLength}
}
