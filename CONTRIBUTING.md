# 贡献指南

感谢你愿意参与 vango。以下约定帮助我们高效地评审与合并改动。

## 环境

- Go 1.22 或更高版本
- `make`（可选，Makefile 封装了常用命令）

## 开发流程

```bash
git clone https://github.com/funxdata/vango.git
cd vango

make check      # gofmt + go vet + 单元测试
make test-race  # 竞态检测
```

## 提交前

1. 运行 `make fmt` 与 `make check`，确保仓库无格式差异、无 vet 告警、测试全绿。
2. 新增或修改行为时补充测试，测试文件与被测代码同包同目录。
3. 公开 API 必须有 godoc 注释，注释以标识符开头。
4. 更新 `CHANGELOG.md` 的 `Unreleased` 段落。

## 依赖策略

vango 的核心承诺是**零第三方依赖**。新增依赖的 PR 需要说明：

- 标准库为什么无法满足；
- 该依赖的维护状态、许可证与安全记录；
- 是否可以放到独立模块，而不进入核心包。

## 提交信息

采用约定式提交：

```
feat(web): 支持路由分组中间件
fix(auth): 修正 JWT 过期边界
docs: 补充配置优先级说明
```

## 代码风格

- 遵循 `gofmt`；命名使用简短、贴近 Go 习惯的英文。
- 注释与文档使用中文，公开 API 的错误信息统一为英文，
  便于调用方在日志与监控中检索。
- 导出类型不返回裸 `error` 之外的内部实现细节；对客户端可见的错误使用 `errs.Error`。

## 分支与发布

- `main` 始终可发布。
- 发布使用语义化版本标签：`vMAJOR.MINOR.PATCH`。
- 破坏性变更必须在 CHANGELOG 中标注 `BREAKING`。
