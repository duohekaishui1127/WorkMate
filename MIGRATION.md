# V7.1 -> V7.1.2

无需迁移数据。V7.1.2 仅修复托盘回调解析与右键菜单，仍沿用 `%APPDATA%\WorkMate`。

# V7.0 -> V7.1

V7.1 沿用 `%APPDATA%\WorkMate` 数据目录，不需要迁移用户数据。

变化：

- 单实例 Mutex 改为稳定名称 `Local\WorkMate.SingleInstance`。
- 同时兼容 V7.0 Mutex，升级期间不会出现 V7.0 / V7.1 双进程。
- 正式安装方式切换为 Inno Setup 6。
- 新增 Windows manifest / 图标 / 文件版本资源构建流程。
- 新增 Authenticode 签名与验签脚本。

建议升级前从托盘“退出 WorkMate”，再运行新的标准安装程序。

## V7.1.2

- 修复单实例锁的 Win32 last-error 判断方式：直接读取 `CreateMutexW` 调用返回的 `ERROR_ALREADY_EXISTS`，不再事后调用 `GetLastError`。
- Mutex 以 initial-owner 方式创建，并继续同时占用稳定名称和 V7 兼容名称。
- 如果已启用开机自启动，每次启动都会把 `HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run\\WorkMate` 修复为当前 WorkMate.exe 路径，避免旧测试版路径在下次登录时再次启动。
- 如果 Windows 通知区域短暂显示一个旧图标，但任务管理器只有一个 WorkMate.exe，把鼠标移到旧图标上后应自动消失；这是 Explorer 的通知区缓存残影，不是第二个进程。
- 托盘图标改用固定 GUID 标识，并在添加前先按 GUID 清理旧条目；从 V7.1.2 起即使 Explorer/程序异常退出，也不应持续积累重复托盘图标。
