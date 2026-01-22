package sys

import (
	"os"
	"slices"
	"strings"
)

func Environ() EnvSet {
	return EnvSet(os.Environ()).Compact()
}

type EnvSet []string

func (e EnvSet) Set(key, value string) EnvSet {
	return append(e.Del(key), key+"="+value)
}

func (e EnvSet) Del(keys ...string) EnvSet {
	return slices.DeleteFunc(e, func(item string) bool {
		k, _, _ := strings.Cut(item, "=")
		return slices.Contains(keys, k)
	})
}

func (e EnvSet) Find(key string) (value string, find bool) {
	for _, item := range slices.Backward(e) {
		k, v, _ := strings.Cut(item, "=")
		if k == key {
			return v, true
		}
	}
	return "", false
}

func (e EnvSet) Get(key string) string {
	v, _ := e.Find(key)
	return v
}

func (e EnvSet) Compact() EnvSet {
	seen := map[string]bool{}
	for i, item := range slices.Backward(e) {
		k, _, _ := strings.Cut(item, "=")
		if seen[k] {
			e = slices.Delete(e, i, i+1)
		} else {
			seen[k] = true
		}
	}
	return e
}

func (e EnvSet) Sets(key_value_pairs ...string) EnvSet {
	var keys = make([]string, 0, len(key_value_pairs)/2)
	for i := 0; i < len(key_value_pairs); i += 2 {
		keys = append(keys, key_value_pairs[i])
	}
	e = e.Del(keys...)
	for i := 0; i < len(key_value_pairs); i += 2 {
		e = append(e, key_value_pairs[i]+"="+key_value_pairs[i+1])
	}
	return e
}
