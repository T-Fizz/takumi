package config

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// decodeStrict parses YAML into target, rejecting unknown fields.
// When yaml.v3 reports unknown fields, the error is rewritten to include
// "did you mean" suggestions for fields that are close to a known one.
func decodeStrict(data []byte, target any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(target); err != nil {
		return enrichUnknownFieldError(err, target)
	}
	return nil
}

// yaml.v3 KnownFields error lines look like:
//   "line 5: field cacheable not found in type config.Phase"
var unknownFieldRe = regexp.MustCompile(`line (\d+): field (\S+) not found in type (\S+)`)

func enrichUnknownFieldError(err error, root any) error {
	matches := unknownFieldRe.FindAllStringSubmatch(err.Error(), -1)
	if len(matches) == 0 {
		return err
	}

	lines := make([]string, 0, len(matches))
	for _, m := range matches {
		line, field, typeName := m[1], m[2], m[3]
		known := knownFieldsForType(root, typeName)
		base := fmt.Sprintf("line %s: unknown field %q in %s", line, field, friendlyTypeName(typeName))
		if suggestion := nearestField(field, known); suggestion != "" {
			base += fmt.Sprintf(" (did you mean %q?)", suggestion)
		}
		lines = append(lines, base)
	}
	return fmt.Errorf("%s", strings.Join(lines, "\n"))
}

// knownFieldsForType walks the reachable types from root looking for a struct
// whose name matches the unqualified name in fullTypeName (e.g. "config.Phase"
// → "Phase"), and returns its yaml-tagged field names.
func knownFieldsForType(root any, fullTypeName string) []string {
	want := fullTypeName
	if i := strings.LastIndex(want, "."); i >= 0 {
		want = want[i+1:]
	}
	t := reflect.TypeOf(root)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return findFieldsByTypeName(t, want, make(map[reflect.Type]bool))
}

func findFieldsByTypeName(t reflect.Type, want string, seen map[reflect.Type]bool) []string {
	if seen[t] {
		return nil
	}
	seen[t] = true
	if t.Kind() == reflect.Struct && t.Name() == want {
		return yamlFieldNames(t)
	}
	switch t.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Array, reflect.Map:
		return findFieldsByTypeName(t.Elem(), want, seen)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			if found := findFieldsByTypeName(t.Field(i).Type, want, seen); len(found) > 0 {
				return found
			}
		}
	}
	return nil
}

func yamlFieldNames(t reflect.Type) []string {
	names := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// nearestField returns the closest candidate within an edit-distance threshold,
// or "" if nothing is close enough.
func nearestField(input string, candidates []string) string {
	threshold := max(2, len(input)/3)
	best := ""
	bestDist := -1
	for _, c := range candidates {
		d := levenshtein(input, c)
		if d <= threshold && (bestDist == -1 || d < bestDist) {
			best = c
			bestDist = d
		}
	}
	return best
}

func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	curr := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		curr[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}
	return prev[len(br)]
}

func friendlyTypeName(t string) string {
	if i := strings.LastIndex(t, "."); i >= 0 {
		return t[i+1:]
	}
	return t
}
