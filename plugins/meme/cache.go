package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (m *MemePlugin) loadCache() error {
	resp, err := http.Get(m.apiURL("/meme/infos"))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("meme-api 返回异常状态码: %d", resp.StatusCode)
	}

	var infos []*memeInfo
	if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
		return err
	}

	cache := make(map[string]*memeInfo, len(infos)*2)
	for _, info := range infos {
		cache[info.Key] = info
		for _, kw := range info.Keywords {
			cache[kw] = info
		}
	}

	m.mu.Lock()
	m.cache = cache
	m.infos = infos
	m.mu.Unlock()
	return nil
}

func (m *MemePlugin) lookupByKeyword(keyword string) *memeInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cache[keyword]
}

func (m *MemePlugin) getInfoList() []*memeInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.infos
}
