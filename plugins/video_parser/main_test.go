package main

import (
	"fmt"
	"testing"

	"github.com/wujunwei928/parse-video/parser"
)

func TestDouyinVideo(t *testing.T) {
	rawText := `6.41 11/28 a@A.GI aAt:/ :8pm 复制打开抖音极速版，看看【張震嶽的作品】谢谢昨晚的悉尼，很开心。 下一站墨尔本，明天晚上 ... https://v.douyin.com/hXn8sT_DJL4/`
	info, err := parser.ParseVideoShareUrlByRegexp(rawText)
	if err != nil {
		t.Fatalf("抖音视频解析失败: %v", err)
	}
	fmt.Printf("\n=== [抖音视频解析结果] ===\n")
	fmt.Printf("标题: %s\n", info.Title)
	fmt.Printf("作者: %s\n", info.Author.Name)
	fmt.Printf("直链 (长度 %d): %s\n", len(info.VideoUrl), info.VideoUrl)
	fmt.Printf("封面: %s\n", info.CoverUrl)
	fmt.Printf("图片数: %d\n", len(info.Images))
}

func TestRedBookImages(t *testing.T) {
	rawText := `人要有一颗独立思考的脑子！！！ https://xhslink.cn/o/3XqcY90fWih 来【小红书】逛逛这篇笔记吧~`
	info, err := parser.ParseVideoShareUrlByRegexp(rawText)
	if err != nil {
		t.Fatalf("小红书解析失败: %v", err)
	}
	fmt.Printf("\n=== [小红书解析结果] ===\n")
	fmt.Printf("标题: %s\n", info.Title)
	fmt.Printf("作者: %s\n", info.Author.Name)
	fmt.Printf("直链: %s\n", info.VideoUrl)
	fmt.Printf("封面: %s\n", info.CoverUrl)
	fmt.Printf("图片数: %d\n", len(info.Images))
	for i, img := range info.Images {
		fmt.Printf("  [%d] %s\n", i+1, img.Url)
	}
}
