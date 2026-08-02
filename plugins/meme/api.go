package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (m *MemePlugin) uploadImage(avatarURL string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"type": "url",
		"url":  avatarURL,
	})
	resp, err := http.Post(m.apiURL("/image/upload"), "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ImageID string `json:"image_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.ImageID == "" {
		return "", fmt.Errorf("上传图片失败: 空 image_id")
	}
	return result.ImageID, nil
}

func (m *MemePlugin) generateMeme(key string, images []map[string]string, texts []string) (string, error) {
	payload := map[string]any{
		"images":  images,
		"texts":   texts,
		"options": map[string]any{},
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(m.apiURL("/memes/"+key), "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(respBody, &errResp)
		if errResp.Message != "" {
			return "", fmt.Errorf("%s", errResp.Message)
		}
		return "", fmt.Errorf("生成失败 (status: %d)", resp.StatusCode)
	}

	var result struct {
		ImageID string `json:"image_id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	return result.ImageID, nil
}

func (m *MemePlugin) downloadImage(imageID string) ([]byte, error) {
	resp, err := http.Get(m.apiURL("/image/" + imageID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
