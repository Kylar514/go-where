package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

func parseSettings(data []byte) (hujson.Value, error) {
	// HuJSON aliases its input; standardization/patches must not mutate the
	// original bytes used for comment preservation and stale-write checks.
	v, err := hujson.Parse(bytes.Clone(data))
	if err != nil {
		return v, err
	}
	if _, ok := v.Value.(*hujson.Object); !ok {
		return v, fmt.Errorf("settings must be a JSON object")
	}
	// encoding/json accepts duplicate names, but AST edits would target an
	// ambiguous occurrence. Reject them before reading or changing settings.
	var check func(hujson.Value) error
	check = func(v hujson.Value) error {
		switch x := v.Value.(type) {
		case *hujson.Object:
			seen := map[string]bool{}
			for _, m := range x.Members {
				name := m.Name.Value.(hujson.Literal).String()
				if seen[name] {
					return fmt.Errorf("duplicate settings key %q", name)
				}
				seen[name] = true
				if err := check(m.Value); err != nil {
					return err
				}
			}
		case *hujson.Array:
			for _, e := range x.Elements {
				if err := check(e); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return v, check(v)
}

func settingsPointer(path []string) string {
	p := make([]string, len(path))
	for i, k := range path {
		p[i] = strings.NewReplacer("~", "~0", "/", "~1").Replace(k)
	}
	return "/" + strings.Join(p, "/")
}

func setSetting(v *hujson.Value, path []string, value any) error {
	for i := 1; i < len(path); i++ {
		ptr := settingsPointer(path[:i])
		parent := v.Find(ptr)
		if parent == nil {
			if err := patchSetting(v, "add", ptr, map[string]any{}); err != nil {
				return err
			}
		} else {
			if _, ok := parent.Value.(*hujson.Object); !ok {
				literal, ok := parent.Value.(hujson.Literal)
				if !ok || string(literal) != "null" {
					return fmt.Errorf("%s is not an object", ptr)
				}
				empty, _ := hujson.Parse([]byte("{}"))
				parent.Value = empty.Value
			}
		}
	}
	ptr := settingsPointer(path)
	if existing := v.Find(ptr); existing != nil {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		next, err := hujson.Parse(data)
		if err != nil {
			return err
		}
		existing.Value = next.Value // Keep comments/whitespace around the value.
		return nil
	}
	return patchSetting(v, "add", ptr, value)
}

func patchSetting(v *hujson.Value, op, path string, value any) error {
	data, err := json.Marshal([]map[string]any{{"op": op, "path": path, "value": value}})
	if err != nil {
		return err
	}
	return v.Patch(data)
}

// All local-settings writers share this path. Preserve source-file symlinks,
// comments and unrelated fields; reject stale drafts instead of overwriting.
func modifySettings(path string, expected []byte, edit func(*hujson.Value, Config) error) error {
	if databaseSettings(path) {
		return modifyDatabaseSettings(path, expected, edit)
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("settings are locked (%s): %w", path+".lock", err)
	}
	lock.Close()
	defer os.Remove(path + ".lock")
	original, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if expected != nil && !bytes.Equal(original, expected) {
		return fmt.Errorf("settings changed while editing; reopen Settings and retry")
	}
	data := original
	if len(data) == 0 && os.IsNotExist(err) {
		data = []byte("{}")
	}
	c, err := decodeConfig(data)
	if err != nil {
		return err
	}
	v, err := parseSettings(data)
	if err != nil {
		return err
	}
	if err = edit(&v, c); err != nil {
		return err
	}
	result := v.Pack()
	if _, err = decodeConfig(result); err != nil {
		return err
	}
	if bytes.Equal(original, result) {
		return nil
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".go-where-settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(result); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !bytes.Equal(original, current) {
		return fmt.Errorf("settings changed during save; retry")
	}
	return os.Rename(f.Name(), path)
}

func saveSettingsDraft(path string, original []byte, before, after Config) error {
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	var old, next any
	_ = json.Unmarshal(a, &old)
	_ = json.Unmarshal(b, &next)
	return modifySettings(path, original, func(v *hujson.Value, _ Config) error {
		var apply func([]string, any, any) error
		apply = func(path []string, a, b any) error {
			if reflect.DeepEqual(a, b) {
				return nil
			}
			am, aok := a.(map[string]any)
			bm, bok := b.(map[string]any)
			if bok {
				if !aok {
					am = map[string]any{}
				}
				keys := map[string]bool{}
				for k := range am {
					keys[k] = true
				}
				for k := range bm {
					keys[k] = true
				}
				ordered := make([]string, 0, len(keys))
				for k := range keys {
					ordered = append(ordered, k)
				}
				sort.Strings(ordered)
				if len(bm) == 0 {
					return setSetting(v, path, bm)
				}
				for _, k := range ordered {
					p := append(append([]string{}, path...), k)
					if _, ok := bm[k]; !ok {
						if v.Find(settingsPointer(p)) != nil {
							if err := patchSetting(v, "remove", settingsPointer(p), nil); err != nil {
								return err
							}
						}
					} else {
						if err := apply(p, am[k], bm[k]); err != nil {
							return err
						}
					}
				}
				return nil
			}
			return setSetting(v, path, b)
		}
		return apply(nil, old, next)
	})
}
