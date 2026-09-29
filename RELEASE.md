# WorkMate V8 正式发布流程

## 1. 构建主程序

运行：

```bat
scripts\build-app.cmd
```

它会生成 `dist\WorkMate.exe`，并通过 go-winres 嵌入：

- WorkMate 图标
- Windows manifest
- 文件版本 / 产品版本
- ProductName / CompanyName / FileDescription

## 2. 构建标准安装器

安装 Inno Setup 7/6 后运行：

```bat
scripts\build-installer.cmd
```

得到：

```text
dist\WorkMate-Setup-0.8.0.exe
```

这个安装器使用固定 AppId，后续 0.7.2 / 0.8.0 会被 Windows 视为同一个产品并进行覆盖升级。

## 3. 代码签名

见 `SIGNING.md`。

## 4. 发布前检查

- 双击 WorkMate 两次，只能看到一个 WorkMate 进程。
- 第二次双击应唤醒已经运行的窗口。
- 安装器能覆盖旧版本。
- 卸载后 `%APPDATA%\WorkMate` 默认保留。
- `signtool verify /pa /all /v` 通过。
- Windows Defender 无告警后再发给其他用户。
