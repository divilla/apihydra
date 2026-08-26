package execution

import (
    "fmt"
    "strings"
)

type Cookie struct {
    store *CookieKeyValueStore
}

func NewCookie(store *CookieKeyValueStore) *Cookie {
    return &Cookie{store: store}
}

func (c *Cookie) LoadByIncluded(keys []string) string {
    var jar string
    for key, val := range c.store.GetAll() {
        if contains(keys, key) {
            jar += fmt.Sprintf("%s\n", val)
        }
    }
    return jar
}

func (c *Cookie) LoadByExcluded(keys []string) string {
    var jar string
    for key, val := range c.store.GetAll() {
        if !contains(keys, key) {
            jar += fmt.Sprintf("%s\n", val)
        }
    }
    return jar
}

func (c *Cookie) Save(jar string) {
    for line := range strings.Lines(jar) {
        vals := strings.Fields(line)
        c.store.Set(makeCookieKey(vals[5], vals[0], vals[1]), line)
    }
}
