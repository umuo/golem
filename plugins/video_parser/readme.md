# Video Parser 插件

视频在线解析插件，自动识别消息中的视频分享链接并解析为可直接播放的视频地址。

## 📋 功能特性

- **自动识别**：自动检测消息中的视频分享链接
- **多平台支持**：支持抖音、快手、视频号等主流短视频平台
- **链接解析**：将分享链接解析为真实的视频播放地址
- **零配置**：开箱即用，无需额外配置
- **信息提取**：自动提取视频标题、作者、封面等信息
- **卡片展示**：以链接卡片形式发送解析结果

## 🚀 快速开始

### 启用插件

将插件编译后放入 `plugins` 目录，重启 Golem 即可自动加载。

### 使用方式

直接在聊天中发送包含视频分享链接的消息：

```
# 抖音分享链接
https://v.douyin.com/xxxxx/

# 快手分享链接
https://v.kuaishou.com/xxxxx

# 小红书分享链接
https://xhslink.cn/xxxxx
```

### 使用方式

直接在聊天中发送包含视频分享链接的消息：

```
# 抖音分享链接
https://v.douyin.com/xxxxx/

# 快手分享链接
https://v.kuaishou.com/xxxxx

# 小红书分享链接
https://xhslink.cn/xxxxx
```

插件会自动识别并解析：
- **视频**：直接发送视频卡片（包含标题、作者、直链、封面），若发送失败自动降级为文本直链
- **图文笔记**：发送首张封面大图 + 编号原图直链清单（防刷屏）

## 🎯 工作原理

### 处理流程

1. **消息监听**：监听所有文本消息事件
2. **链接识别**：使用正则表达式识别消息中的视频分享链接
3. **视频解析**：调用解析库获取真实视频地址和相关信息
4. **结果发送**：将解析结果以链接卡片形式发送给用户

### 消息处理逻辑

```
文本消息
    ↓
检测是否包含视频链接
    ↓
    有 → 解析视频信息
         ↓
         发送链接卡片（标题、作者、视频地址、封面）
    ↓
    无 → 忽略消息，不处理
```

## 🔧 支持的平台

基于 [parse-video](https://github.com/wujunwei928/parse-video) 库实现，支持以下平台：

### 抖音（Douyin）
- 分享链接格式：`https://v.douyin.com/xxxxx/`
- 提取信息：视频标题、作者名称、视频地址、封面图

### 快手（Kuaishou）
- 分享链接格式：`https://v.kuaishou.com/xxxxx`
- 提取信息：视频标题、作者名称、视频地址、封面图

### 视频号（Channels）
- 分享链接格式：`https://channels.weixin.qq.com/xxxxx`
- 提取信息：视频标题、作者名称、视频地址、封面图

### 其他平台
更多平台支持请参考 [parse-video 项目文档](https://github.com/wujunwei928/parse-video)。

## 📝 返回信息格式

解析成功后，插件会发送一个链接卡片消息，包含以下信息：

| 字段 | 说明 | 示例 |
|------|------|------|
| 标题（Title） | 视频标题 | `搞笑视频合集` |
| 描述（Desc） | 作者名称 | `张三` |
| 链接（Url） | 真实视频播放地址 | `https://example.com/video.mp4` |
| 封面（Xml） | 视频封面图地址 | `https://example.com/cover.jpg` |

## 💡 使用场景

### 1. 视频分享助手

**场景**：用户分享短视频链接，自动解析为可直接观看的地址

```
用户：快看这个视频 https://v.douyin.com/xxxxx/
      ↓
插件：[视频标题]
      作者：xxx
      [视频播放链接]
```

### 2. 视频收藏

**场景**：保存视频真实地址，避免分享链接失效

```
原始链接：https://v.douyin.com/xxxxx/（可能失效）
      ↓
真实地址：https://cdn.example.com/video.mp4（长期有效）
```

### 3. 跨平台分享

**场景**：将短视频平台的链接转换为通用播放地址

```
平台分享链接 → 真实视频地址 → 在其他应用中播放
```

## ⚙️ 配置说明

### 插件元数据

| 配置项 | 值 | 说明 |
|--------|-----|------|
| `Name` | `video_parser` | 插件名称 |
| `Author` | `ovo` | 作者 |
| `Version` | `v1.0.0` | 版本号 |
| `Priority` | `100` | 优先级（较高） |
| `Next` | `false` | 处理后不继续传递事件 |
| `AlwaysRun` | `false` | 仅在匹配时运行 |

### 订阅的事件

- `message::text`：文本消息事件

### 依赖项

| 依赖 | 版本 | 说明 |
|------|------|------|
| `github.com/sbgayhub/golem/sdk` | v0.1.1 | Golem SDK |
| `github.com/wujunwei928/parse-video` | v0.0.2 | 视频解析库 |

## 📌 技术细节

### 链接识别机制

插件使用正则表达式识别视频分享链接：

```go
// parse-video 库会自动识别以下模式
// - https://v.douyin.com/xxxxx/
// - https://v.kuaishou.com/xxxxx
// - https://channels.weixin.qq.com/xxxxx
// 等主流短视频平台分享链接
```

### 解析流程

```go
// 1. 调用解析库
info, err := parser.ParseVideoShareUrlByRegexp(msg.Content)

// 2. 提取视频信息
// - info.Title: 视频标题
// - info.Author.Name: 作者名称
// - info.VideoUrl: 视频播放地址
// - info.CoverUrl: 封面图地址

// 3. 发送链接卡片消息
message.Send(&message.Message{
    Type: message.TypeAppLink,
    Data: &message.AppData{
        SubType: 5,  // 链接卡片类型
        Title:   info.Title,
        Desc:    info.Author.Name,
        Url:     info.VideoUrl,
        Xml:     info.CoverUrl,
    },
})
```

### 错误处理

| 错误类型 | 处理方式 |
|----------|----------|
| `str not have url` | 消息中不包含视频链接，忽略消息 |
| 解析失败 | 记录警告日志，返回错误 |
| 发送失败 | 记录警告日志，不阻塞后续处理 |

## 🔍 故障排查

### 插件不响应

**可能原因**：
1. 插件未正确加载
2. 消息中不包含可识别的视频链接
3. 链接格式不正确

**解决方案**：
1. 检查插件是否已加载：查看日志输出
2. 确认链接格式：使用标准的平台分享链接
3. 查看日志：检查是否有错误信息

### 解析失败

**可能原因**：
1. 视频链接已失效
2. 平台更新了接口或反爬虫策略
3. 网络连接问题

**解决方案**：
1. 尝试使用新的分享链接
2. 更新 `parse-video` 库到最新版本
3. 检查网络连接和防火墙设置

### 返回信息不完整

**可能原因**：
1. 视频已被删除或下架
2. 作者设置了隐私权限
3. 平台接口返回数据不完整

**解决方案**：
1. 确认视频在原平台可以正常访问
2. 查看日志中的具体错误信息
3. 反馈给 `parse-video` 库维护者

## 🎨 示例场景

### 场景 1：日常分享

```
用户A：这个视频太搞笑了 https://v.douyin.com/xxxxx/
      ↓
插件：[搞笑日常 | 张三的日常]
      作者：张三
      视频地址：https://cdn.douyin.com/video/12345.mp4
      [封面图]
      ↓
用户B：确实哈哈哈
```

### 场景 2：内容收藏

```
用户：收藏这个教程 https://v.douyin.com/xxxxx/
      ↓
插件：[Go语言入门教程第一课]
      作者：编程小王子
      视频地址：https://cdn.douyin.com/video/67890.mp4
      [封面图]
      ↓
用户：保存到笔记 [使用真实地址，不怕分享链接失效]
```

### 场景 3：群聊讨论

```
用户A：大家看看这个 https://v.kuaishou.com/xxxxx
        ↓
插件：  [美食制作：如何做红烧肉]
        作者：美食达人
        视频地址：https://cdn.kuaishou.com/video/abcde.mp4
        [封面图]
        ↓
用户B： 这个做法不错
用户C： 收藏了
```

## 🚧 已知限制

1. **平台限制**：仅支持 `parse-video` 库已实现的平台
2. **链接时效**：解析后的视频地址可能存在时效性（取决于平台 CDN 策略）
3. **反爬虫**：部分平台可能有反爬虫机制，导致解析失败
4. **网络依赖**：需要网络连接才能解析视频信息
5. **无命令接口**：插件为自动触发型，不提供手动命令

## 🔮 未来计划

- [ ] 支持更多视频平台（B站、西瓜视频等）
- [ ] 添加视频下载功能
- [ ] 支持自定义解析规则
- [ ] 提供命令行接口手动触发解析
- [ ] 缓存解析结果，减少重复请求
- [ ] 支持批量解析多个视频链接

## 🤝 相关项目

- [parse-video](https://github.com/wujunwei928/parse-video) - 视频解析核心库
- [Golem](https://github.com/sbgayhub/golem) - 机器人框架

## 📄 许可证

遵循主项目许可证。

## 🙏 致谢

感谢 [wujunwei928/parse-video](https://github.com/wujunwei928/parse-video) 提供的视频解析能力。

---

**作者**：ovo  
**版本**：v1.0.0