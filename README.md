# China Eyeball Prefixes

中国大陆三大基础电信运营商的普通互联网用户（eyeball）IPv4/IPv6
前缀列表。公开产物按运营商和省级行政区划组织，不公开拆分固网、移网
列表。

## 公开文件

```text
lists/
├── ipv4/
│   ├── cn.txt
│   ├── chinatelecom.txt
│   ├── chinamobile.txt
│   ├── chinaunicom.txt
│   └── provinces/
│       ├── anhui.txt
│       ├── ...
│       └── zhejiang.txt
├── ipv6/
│   ├── cn.txt
│   ├── chinatelecom.txt
│   ├── chinamobile.txt
│   ├── chinaunicom.txt
│   └── provinces/
│       ├── anhui.txt
│       ├── ...
│       └── zhejiang.txt
└── manifest.json
```

- `lists/<family>/cn.txt`：该地址族三家运营商列表的并集。
- `lists/<family>/<operator>.txt`：单一运营商的全国列表。
- `lists/<family>/provinces/<province>.txt`：该省三家运营商的合并列表。
- `lists/manifest.json`：全部公开文件的统一 schema、CIDR 数量和 SHA-256。

每个 TXT 文件均为 UTF-8、每行一个规范 CIDR，不含表头和注释。运营商
文件直接位于地址族目录下，没有 `operators/` 二级目录。

省份表示地址规划和网络路由的省级归属，不等同于终端的实时物理位置。
尤其在移动网络漫游、跨省核心网锚定或集中出口场景中，两者可能不同。

## 范围和生成原则

目标范围是中国电信、中国移动和中国联通的普通公众互联网接入前缀。
生成器以当前 BGP Origin、运营商登记和用途证据为基础，尽量排除明确的
云计算、IDC、CDN、专线、政企专网及其他非普通终端接入资源。

IPv4 主要结合：

- 当前 BGP Origin 与 ASN 描述；
- APNIC `inetnum`、`aut-num`、`organisation` 和 `route` 登记；
- RIPE RIS 的当前路由与强 MOAS 证据；
- 独立云厂商前缀和省级定位数据。

IPv6 主要结合：

- RIPE RIS 当前可见的精确 BGP 前缀；
- IPtoASN 的 Origin 元数据；
- APNIC `inet6num` 登记；
- 经审计的运营商 IPv6 省级地址规划配置。

IPv4 与 IPv6 使用各自的准入策略。IPv6 不因 Origin 是 AS4809
或 AS9929 就单独排除已经落在确认接入地址规划内的前缀。精确 BGP
宣告中的父子前缀会在公开文件中折叠为地址集合等价的最小 CIDR；
原始宣告单元只保留在当次审计中。

固网/移网属性只在生成期用于地址规划匹配和审计，不作为公开文件维度，
也不能单独证明某个前缀一定属于普通终端。承载 ASN、地址规划用途与终端
业务属性是三个不同概念。

本项目是可审计的 best-effort allowlist，不是用户实时定位数据库，也不
保证覆盖运营商未公开宣告、临时撤回或刚刚启用的地址。

## 自动更新和审计

`.github/workflows/update.yml` 每日下载上游数据，在隔离的 staging 目录中
生成并校验 IPv4、IPv6 列表。两种地址族都通过校验后，TXT 文件才会发布
到 `lists/`，随后生成单一的 `lists/manifest.json`。

每次运行都会上传审计 artifact，其中包含：

- IPv4、IPv6 生成和验证日志；
- IPv4 详细 manifest 与 IPv6 审计 manifest；
- 上游源文件 SHA-256；
- 当次公开 manifest、Git 状态和列表差异。

详细审计数据不作为公开列表提交。只有公开列表内容或 manifest schema
发生变化时，工作流才会提交 `lists/`；时间戳或审计元数据单独变化不会
产生提交。

## 本地验证

Go 模块位于 `generator/`：

```bash
go -C generator test ./...
go -C generator vet ./...
```

工作流使用以下统一子命令：

```bash
go -C generator run ./cmd/generate ipv4 ...
go -C generator run ./cmd/generate ipv6 ...
go -C generator run ./cmd/verify ipv4 ...
go -C generator run ./cmd/verify ipv6 ...
go -C generator run ./cmd/generate manifest --root ../lists
```

完整参数和上游文件名以
[更新工作流](.github/workflows/update.yml) 为准。生成器先写 staging，
验证成功后才同步到公开目录，避免失败运行留下半成品。

## 路径兼容性

当前公开契约是 `lists/{ipv4,ipv6}`。旧版 `data/`、`data/ipv6/` 以及
固网/移网研究列表不再作为活动发布路径。依赖旧路径的消费者应固定到
迁移前的版本，或更新为上述路径；主分支不保留重复列表或符号链接。
