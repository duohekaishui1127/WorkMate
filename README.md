# 打工搭子 WorkMate V7.1.2 Native

Windows 本地优先的打工状态 / 劳动日历 / 加班记录小工具。

## V7.1.2 托盘菜单修复

- 修复 Windows 10/11 下托盘图标右键无菜单的问题。原因是 `NOTIFYICON_VERSION_4` 会把通知事件放进 `LOWORD(lParam)`，旧代码错误地比较了完整的 `lParam`。
- 同时兼容 `WM_RBUTTONUP` 与 `WM_CONTEXTMENU`。
- 托盘菜单现在提供：打开 WorkMate、显示/隐藏桌面挂件、开始/结束加班、老板来了、设置、完全退出 WorkMate。
- `完全退出 WorkMate` 会走统一退出流程：注销全局快捷键、删除托盘图标、保存本地数据并结束唯一进程。


## V7.1 重点

### 1. 真正的单实例

WorkMate 使用 Windows Named Mutex 保证同一用户会话中只能运行一个实例。

- 第一次启动：创建并持有单实例 Mutex。
- 第二次双击：不会启动第二个 WorkMate 进程，而是唤醒已运行的主窗口。
- 同时兼容 V7.0 的旧 Mutex，避免升级过程中 V7.0 和 V7.1 并行运行。
- Mutex 名保持稳定，后续版本继续复用。

### 2. 正规安装器工程

V7.1 不再使用自制 Go Setup 作为正式发行方案。

正式安装包由 `installer/WorkMate.iss` 使用 **Inno Setup 7/6** 生成，支持：

- 固定 AppId，后续版本覆盖升级
- 当前用户安装，不强制管理员权限
- 标准开始菜单 / 桌面快捷方式
- 标准 Windows 卸载项
- 安装完成启动
- 安装时请求关闭正在运行的 WorkMate
- 安装器版本信息

### 3. Windows 资源信息

构建脚本通过 `go-winres` 给 `WorkMate.exe` 嵌入：

- 正式图标
- Windows manifest
- 文件版本 / 产品版本
- ProductName / FileDescription / CompanyName

### 4. 代码签名流程

已经加入：

- `SIGNING.md`
- `scripts/sign-release.cmd`
- `scripts/verify-signature.cmd`

证书和私钥**不会**放进源码。获得受信任 Authenticode 证书后即可接入发布流程。

## 构建

Windows 安装 Go 1.23+ 后：

```bat
scripts\build-app.cmd
```

第一次会安装 `go-winres v0.3.3`。

生成：

```text
dist\WorkMate.exe
```

安装 Inno Setup 7/6 后：

```bat
scripts\build-installer.cmd
```

生成：

```text
dist\WorkMate-Setup-0.7.1.exe
```

完整发布流程请看 `RELEASE.md`，代码签名请看 `SIGNING.md`。

## 数据

用户数据仍然默认保存在：

```text
%APPDATA%\WorkMate
```

升级或卸载应用不会主动删除这部分个人数据。

## 安全设计

主程序不依赖 PowerShell、不使用 `ExecutionPolicy Bypass`，没有隐藏脚本启动链。正式发行建议使用标准 Inno Setup 安装包，并同时对主程序和安装包进行 Authenticode 签名。
