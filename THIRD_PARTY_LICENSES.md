# 第三方组件与许可证 / Third-Party Notices

ElectricHamster 本体以 [MIT License](LICENSE) 发布。以下为运行/构建时涉及的第三方组件。

## Go 依赖（MIT / BSD-3-Clause）

| 组件 | 许可证 |
|---|---|
| github.com/wailsapp/wails/v3 | MIT |
| github.com/xuri/excelize/v2 | BSD-3-Clause |
| golang.org/x/crypto | BSD-3-Clause |
| golang.org/x/term | BSD-3-Clause |
| modernc.org/sqlite | BSD-3-Clause |

## 内嵌压缩引擎：7-Zip ZS

发布版可在运行时释放并调用内嵌的 7-Zip ZS 可执行文件（`internal/archive/runtime/<os>-<arch>/`）。
该二进制**不是**本仓库的代码，分发时请一并遵守其许可证：

- 7-Zip 本体：**GNU LGPL**（含 unRAR 部分限制，详见其 `License.txt`）
- 附加 codec：zstd（BSD-3-Clause）、Brotli（MIT）、LZ4（BSD-2-Clause）、Lizard、LZ5、Fast-LZMA2 等，各自许可证见上游

上游项目：<https://github.com/mcmilk/7-Zip-zstd>

> 本项目通过独立进程调用 7-Zip，属于「聚合（aggregation）」而非链接；若在发布产物中附带该二进制，请附上上述许可证文本。

## 旧版

历史版本基于 [zip4j](https://github.com/srikanth-lingala/zip4j)（Apache-2.0）的 Java 实现，相关代码已在本仓库中移除。
