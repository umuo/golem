package main

import (
	"encoding/json"
	"log/slog"
	"math/rand"
	"net/url"
	"strings"
	"time"

	"github.com/sbgayhub/golem/sdk/contact"
)

// handleHelp 发送用法帮助
func (p *SetuPlugin) handleHelp(receiver *contact.Contact) (bool, error) {
	p.sendText(receiver, "【setu 插件】用法：\n"+
		"plmm / 漂亮妹妹 / 来点美女：随机美女图\n"+
		"来点黑丝 / 来点白丝：随机黑丝/白丝（有概率是视频）\n"+
		"看看腿：随机美腿图\n"+
		"来点帅哥：随机帅哥图\n"+
		"来点<关键词>：按关键词搜图（如：来点柯基）")
	return true, nil
}

// handlePlmm 处理漂亮妹妹
func (p *SetuPlugin) handlePlmm(receiver *contact.Contact) (bool, error) {
	slog.Debug("[setu] 处理漂亮妹妹请求", "api", p.Config.ImgURL)

	imgURL, err := p.httpGet(p.Config.ImgURL)
	if err != nil {
		slog.Error("[setu] 获取漂亮妹妹图片失败", "err", err)
		p.sendText(receiver, "获取漂亮妹妹图片失败: "+err.Error())
		return true, nil
	}

	if imgURL == "" {
		slog.Warn("[setu] 漂亮妹妹 API 返回空内容")
		p.sendText(receiver, "获取漂亮妹妹图片失败: API 返回空内容")
		return true, nil
	}

	if !strings.HasPrefix(imgURL, "http://") && !strings.HasPrefix(imgURL, "https://") {
		slog.Warn("[setu] 漂亮妹妹 API 返回非图片地址", "response", imgURL)
		p.sendText(receiver, "获取图片失败: 第三方接口异常或维护中")
		return true, nil
	}

	slog.Debug("[setu] 获取到图片 URL", "url", imgURL)

	if err := p.sendImage(receiver, imgURL); err != nil {
		slog.Error("[setu] 发送漂亮妹妹图片失败", "err", err)
		p.sendText(receiver, "发送图片失败: "+err.Error())
		return true, nil
	}

	slog.Debug("[setu] 发送漂亮妹妹图片成功")
	return true, nil
}

// handleSiImage 处理黑丝/白丝（按概率触发视频，失败降级图片）
func (p *SetuPlugin) handleSiImage(receiver *contact.Contact, name, videoURL, imageURL string) (bool, error) {
	if rand.Intn(100) < p.Config.VideoRate {
		if err := p.sendVideo(receiver, videoURL); err == nil {
			slog.Debug("[setu] 发送视频（概率触发）", "type", name)
			return true, nil
		} else {
			slog.Warn("[setu] 视频失败，降级发送图片", "type", name, "err", err)
		}
	}
	if err := p.sendImage(receiver, imageURL); err != nil {
		slog.Error("[setu] 发送图片失败", "type", name, "err", err)
		p.sendText(receiver, "发送图片失败")
		return true, nil
	}
	slog.Debug("[setu] 发送图片", "type", name)
	return true, nil
}

// handleKkt 处理看看腿（50%黑丝，50%白丝）
func (p *SetuPlugin) handleKkt(receiver *contact.Contact) (bool, error) {
	if time.Now().Unix()%2 == 0 {
		return p.handleSiImage(receiver, "黑丝", p.Config.HeisiVideoURL, p.Config.HeisiURL)
	}
	return p.handleSiImage(receiver, "白丝", p.Config.BaisiVideoURL, p.Config.BaisiURL)
}

// handleBoy 处理来点帅哥
func (p *SetuPlugin) handleBoy(receiver *contact.Contact) (bool, error) {
	imgURL, err := p.httpGet(p.Config.BoyURL)
	if err != nil || imgURL == "" {
		p.sendText(receiver, "获取帅哥图片失败")
		return true, nil
	}

	if !strings.HasPrefix(imgURL, "http://") && !strings.HasPrefix(imgURL, "https://") {
		slog.Warn("[setu] 帅哥 API 返回非图片地址", "response", imgURL)
		p.sendText(receiver, "获取图片失败: 第三方接口异常或维护中")
		return true, nil
	}

	if err := p.sendImage(receiver, imgURL); err != nil {
		slog.Error("[setu] 发送帅哥图片失败", "err", err)
		p.sendText(receiver, "发送图片失败")
		return true, nil
	}

	slog.Debug("[setu] 发送帅哥图片")
	return true, nil
}

// handleSearch 处理来点XX（搜索图片）
func (p *SetuPlugin) handleSearch(receiver *contact.Contact, keyword string) (bool, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return false, nil
	}

	slog.Debug("[setu] 搜索图片关键词", "keyword", keyword)

	// URL 编码
	encodedKeyword := url.QueryEscape(keyword)
	searchURL := p.Config.SearchURL + encodedKeyword

	// 调用搜索 API
	response, err := p.httpGet(searchURL)
	if err != nil || response == "" {
		p.sendText(receiver, "搜索失败，请稍后再试")
		return true, nil
	}

	// 解析 JSON 响应
	var result struct {
		Code int      `json:"code"`
		Msg  string   `json:"msg"`
		Res  []string `json:"res"`
	}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		p.sendText(receiver, "解析响应失败")
		return true, nil
	}

	if result.Code != 200 {
		msg := result.Msg
		if msg == "" {
			msg = "搜索失败"
		}
		p.sendText(receiver, msg)
		return true, nil
	}

	if len(result.Res) == 0 {
		p.sendText(receiver, "没有找到相关图片")
		return true, nil
	}

	// 随机选择一张图片
	index := rand.Intn(len(result.Res))
	imageURL := result.Res[index]

	if err := p.sendImage(receiver, imageURL); err != nil {
		slog.Error("[setu] 发送搜索图片失败", "err", err)
		p.sendText(receiver, "发送图片失败")
		return true, nil
	}

	slog.Debug("[setu] 发送搜索图片成功", "keyword", keyword)
	return true, nil
}
