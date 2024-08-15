// This file contains code from godotenv (https://github.com/joho/godotenv), which is a go port
// of the ruby dotenv library (https://github.com/bkeepers/dotenv), licensed under the MIT license.
//
// Copyright (c) 2023-present Eser Ozvataf
// Copyright (c) 2013 John Barton

package envparser

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/eser/go-service/pkg/bliss/lib"
)

const (
	charComment       = '#'
	prefixSingleQuote = '\''
	prefixDoubleQuote = '"'

	exportPrefix = "export"
)

var (
	ErrZeroLengthString        = errors.New("zero length string")
	ErrKeyNameNotFound         = errors.New("key name not found")
	ErrUnexpectedChar          = errors.New("unexpected character")
	ErrUnterminatedQuotedValue = errors.New("unterminated quoted value")
)

func ParseBytes(data []byte, out *map[string]any) error {
	src := bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	cutset := src

	for {
		cutset = getStatementStart(cutset)
		if cutset == nil {
			// reached end of file
			break
		}

		key, left, err := locateKeyName(cutset)
		if err != nil {
			return err
		}

		value, left, err := extractVarValue(left, out)
		if err != nil {
			return err
		}

		(*out)[key] = value
		cutset = left
	}

	return nil
}

// getStatementPosition returns position of statement begin.
//
// It skips any comment line or non-whitespace character.
func getStatementStart(src []byte) []byte {
	pos := indexOfNonSpaceChar(src)
	if pos == -1 {
		return nil
	}

	src = src[pos:]
	if src[0] != charComment {
		return src
	}

	// skip comment section
	pos = bytes.IndexFunc(src, isCharFunc('\n'))
	if pos == -1 {
		return nil
	}

	return getStatementStart(src[pos:])
}

func extractKeyName(src []byte) (string, int, error) {
	for i, char := range src { //nolint:varnamelen
		rchar := rune(char)
		if !unicode.IsSpace(rchar) && (unicode.IsLetter(rchar) || unicode.IsNumber(rchar) || char == '_' || char == '.') {
			continue
		}

		if char == '=' || char == ':' {
			key := string(src[0:i])

			return key, i + 1, nil
		}

		return "", 0, fmt.Errorf(
			`unexpected character %q in variable name near %q: %w`, string(char), string(src), ErrUnexpectedChar)
	}

	return "", 0, ErrKeyNameNotFound
}

func trimExportPrefix(src []byte) []byte {
	var newSrc []byte

	if bytes.HasPrefix(src, []byte(exportPrefix)) {
		newSrc = bytes.TrimPrefix(src, []byte(exportPrefix))
	} else {
		newSrc = src
	}

	return lib.StringsTrimLeadingSpaceFromBytes(newSrc)
}

// locateKeyName locates and parses key name and returns rest of slice.
func locateKeyName(src []byte) (string, []byte, error) {
	src = trimExportPrefix(src)

	key, offset, err := extractKeyName(src)
	if err != nil {
		return "", nil, err
	}

	if len(src) == 0 {
		return "", nil, ErrZeroLengthString
	}

	key = lib.StringsTrimTrailingSpace(key)
	cutset := lib.StringsTrimLeadingSpaceFromBytes(src[offset:])

	return key, cutset, nil
}

func extractUnquotedVarValue(src []byte, vars *map[string]any) (string, []byte, error) {
	// unquoted value - read until end of line
	endOfLine := bytes.IndexFunc(src, isLineEnd)

	// Hit EOF without a trailing newline
	if endOfLine == -1 {
		endOfLine = len(src)

		if endOfLine == 0 {
			return "", nil, nil
		}
	}

	// Convert line to rune away to do accurate countback of runes
	line := []rune(string(src[0:endOfLine]))

	// Assume end of line is end of var
	endOfVar := len(line)
	if endOfVar == 0 {
		return "", src[endOfLine:], nil
	}

	// Work backwards to check if the line ends in whitespace then
	// a comment (ie asdasd # some comment)
	for i := endOfVar - 1; i >= 0; i-- {
		if line[i] == charComment && i > 0 {
			if isSpace(line[i-1]) {
				endOfVar = i

				break
			}
		}
	}

	trimmed := strings.TrimFunc(string(line[0:endOfVar]), isSpace)

	return expandVariables(trimmed, vars), src[endOfLine:], nil
}

func extractQuotedVarValue(src []byte, vars *map[string]any, quote byte) (string, []byte, error) {
	// lookup quoted string terminator
	for i := 1; i < len(src); i++ { //nolint:varnamelen
		if char := src[i]; char != quote {
			continue
		}

		// skip escaped quote symbol (\" or \', depends on quote)
		if prevChar := src[i-1]; prevChar == '\\' {
			continue
		}

		// trim quotes
		trimFunc := isCharFunc(rune(quote))
		value := string(bytes.TrimLeftFunc(bytes.TrimRightFunc(src[0:i], trimFunc), trimFunc))

		if quote == prefixDoubleQuote {
			// unescape newlines for double quote (this is compat feature)
			// and expand environment variables
			value = expandVariables(expandEscapes(value), vars)
		}

		return value, src[i+1:], nil
	}

	// return formatted error if quoted string is not terminated
	valEndIndex := bytes.IndexFunc(src, isCharFunc('\n'))
	if valEndIndex == -1 {
		valEndIndex = len(src)
	}

	return "", nil, fmt.Errorf("unterminated quoted value %s: %w", src[:valEndIndex], ErrUnterminatedQuotedValue)
}

// extractVarValue extracts variable value and returns rest of slice.
func extractVarValue(src []byte, vars *map[string]any) (string, []byte, error) {
	quote, hasPrefix := hasQuotePrefix(src)

	if !hasPrefix {
		return extractUnquotedVarValue(src, vars)
	}

	return extractQuotedVarValue(src, vars, quote)
}

func expandEscapes(str string) string {
	out := escapeRegex.ReplaceAllStringFunc(str, func(match string) string {
		c := strings.TrimPrefix(match, `\`)
		switch c {
		case "n":
			return "\n"
		case "r":
			return "\r"
		default:
			return match
		}
	})

	return unescapeCharsRegex.ReplaceAllString(out, "$1")
}

func indexOfNonSpaceChar(src []byte) int {
	return bytes.IndexFunc(src, func(r rune) bool {
		return !unicode.IsSpace(r)
	})
}

// hasQuotePrefix reports whether charset starts with single or double quote and returns quote character.
func hasQuotePrefix(src []byte) (byte, bool) {
	if len(src) == 0 {
		return 0, false
	}

	switch prefix := src[0]; prefix {
	case prefixDoubleQuote, prefixSingleQuote:
		return prefix, true
	default:
		return 0, false
	}
}

func isCharFunc(char rune) func(rune) bool {
	return func(v rune) bool {
		return v == char
	}
}

// isSpace reports whether the rune is a space character but not line break character
//
// this differs from unicode.IsSpace, which also applies line break as space.
func isSpace(r rune) bool {
	switch r {
	case '\t', '\v', '\f', '\r', ' ', 0x85, 0xA0: //nolint:gomnd,mnd
		return true
	}

	return false
}

func isLineEnd(r rune) bool {
	if r == '\n' || r == '\r' {
		return true
	}

	return false
}

var (
	escapeRegex        = regexp.MustCompile(`\\.`)
	expandVarRegex     = regexp.MustCompile(`(\\)?(\$)(\()?\{?([A-Z0-9_]+)?\}?`)
	unescapeCharsRegex = regexp.MustCompile(`\\([^$])`)
)

func expandVariables(v string, m *map[string]any) string { //nolint:varnamelen
	return expandVarRegex.ReplaceAllStringFunc(v, func(s string) string { //nolint:varnamelen
		submatch := expandVarRegex.FindStringSubmatch(s)

		if submatch == nil {
			return s
		}

		if submatch[1] == "\\" || submatch[2] == "(" {
			return submatch[0][1:]
		}

		if submatch[4] != "" {
			return (*m)[submatch[4]].(string)
		}

		return s
	})
}

func Parse(m *map[string]any, r io.Reader) error {
	var buf bytes.Buffer

	_, err := io.Copy(&buf, r)
	if err != nil {
		return fmt.Errorf("parsing error: %w", err)
	}

	return ParseBytes(buf.Bytes(), m)
}

func TryParseFiles(m *map[string]any, filenames ...string) error {
	for _, filename := range filenames {
		file, err := os.Open(filename)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return err //nolint:wrapcheck
		}

		defer file.Close()

		err = Parse(m, file)
		if err != nil {
			return err
		}
	}

	return nil
}
