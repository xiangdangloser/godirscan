# godirscan

基于 Go 的并发目录扫描器（目录爆破工具），用于对**已授权目标**进行 Web 路径发现。

## 功能特性

- **Worker Pool 并发模型**：channel 传递任务 + `sync.WaitGroup` 优雅退出，`-c` 参数实时控制并发数
- **扫描统计汇总**：结束输出总请求数、有效路径数、失败/超时数，便于判断「没扫到」还是「全失败」
- **真实响应长度**：`io.Copy` 读取 body 实测长度（`resp.ContentLength` 在分块传输/gzip 时为 -1，不可作为过滤依据）
- **保留原始重定向**：自定义 `CheckRedirect` 不跟随 30x，避免「302 跳登录页最终 200」造成误报
- **User-Agent 伪装**：绕过对 `Go-http-client/1.1` 默认 UA 的简易拦截
- **字典清洗**：自动过滤不可见字符（UTF-16/BOM 残留）与空白行

## 快速开始

```bash
# 方式一：直接运行
go run main.go -u http://127.0.0.1:8080 -w dict.txt -c 30

# 方式二：编译后运行
go build -o godirscan.exe main.go
./godirscan.exe -u http://127.0.0.1:8080 -w raft-small-directories.txt -c 30
```

## 参数说明

| 参数 | 说明 | 默认值 |
|---|---|---|
| `-u` | 目标 URL（必填） | 无 |
| `-w` | 字典文件路径 | `dict.txt` |
| `-c` | 并发协程数 | `50` |

> 注：仓库自带的 `dict.txt` 仅为 4 条演示用小字典（用于快速验证工具是否工作），正式使用请指定 SecLists 字典（见下文「推荐字典」）。

## 输出说明

命中状态码：`200` / `301` / `302` / `403`（403 说明目录存在但禁止访问，同样计入发现）

```
[*] 开始对 http://127.0.0.1:8080/ 进行目录爆破 (并发数: 30)...
[+] 发现路径: http://127.0.0.1:8080/admin            | 状态码: 301 | 长度: 0
[+] 发现路径: http://127.0.0.1:8080/backup           | 状态码: 301 | 长度: 0
[+] 发现路径: http://127.0.0.1:8080/uploads          | 状态码: 301 | 长度: 0
[+] 发现路径: http://127.0.0.1:8080/config           | 状态码: 301 | 长度: 0

[*] 扫描结束！
[*] 总请求数: 20114
[+] 发现有效路径: 4
[-] 请求失败/超时: 18
```

## 实测数据

本地 VMware 靶场（Kali → Ubuntu + `python3 -m http.server`），字典为 SecLists `raft-small-directories.txt`（20115 行）：

| 指标 | 数值 |
|---|---|
| 总请求数 | 20114 |
| 发现路径 | 4（admin / backup / uploads / config，均为 301） |
| 失败/超时 | 18（单线程 dev server 在 30 并发下的连接拒绝，属目标性能瓶颈，非工具缺陷） |

## 核心技术实现与避坑指南

1. **并发模型**：有界 channel 作任务队列，固定数量 worker 消费，`close(jobs)` 广播下班信号，`wg.Wait()` 等全部处理完。相比“每个路径起一个 goroutine”，内存和连接数都可控。
2. **为什么必须 `defer resp.Body.Close()`**：不关闭响应体，底层 TCP 连接不会归还连接池，少量请求无感，上万请求后文件描述符耗尽，表现为扫描器集体超时卡死。
3. **为什么不跟随重定向**：很多站点把不存在路径 302 到登录页，跟随后最终状态码是 200 → 误报。保留原始 30x，把判断权还给使用者。
4. **为什么用 `io.Copy` 测长度**：目录扫描的高级用法是“响应长度差异”过滤通配 404（所有路径都返 200 但内容不同的站），`ContentLength` 在 chunked 传输下是 -1，必须读 body。
5. **计数器用 `sync/atomic`**：多 worker 并发写计数，`int32 + atomic.AddInt32` 避免数据竞争（可用 `go run -race` 验证）。

## 推荐字典

- [SecLists](https://github.com/danielmiessler/SecLists) `Discovery/Web-Content/raft-small-directories.txt`（约 2 万行，日常够用）
- 同目录 `raft-medium-directories.txt`（约 6 万行）/ `raft-large-directories.txt`（约 15 万行），建议工具加入 `-fc`（过滤状态码）/ `-fl`（过滤长度）参数后再使用

## 免责声明

本工具仅用于对**已获得明确书面授权的目标**进行安全测试，或用于个人本地靶场学习。未经授权对他人系统进行扫描可能违反《网络安全法》等法律法规，使用者需自行承担全部责任。

## Roadmap

- [ ] `-o` 结果输出到文件
- [ ] `-fc` / `-fl` 按状态码/长度过滤
- [ ] `-x` 指定扩展名批量探测（.php/.bak/.zip）
- [ ] 失败请求自动重试
- [ ] 递归目录发现（发现目录后自动深入）
