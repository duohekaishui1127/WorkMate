# Security / 发布安全说明

WorkMate V7.1 Native 的主程序为独立 Windows GUI EXE。

## 主程序不会做的事情

- 不启动 PowerShell / cmd / wscript 来承载核心程序。
- 不使用 `ExecutionPolicy Bypass`。
- 不下载并执行远程代码。
- 不读取浏览器密码或 Cookie。
- 不记录键盘输入内容。
- 不上传工资、加班和休假数据。

## 单实例

应用使用 Windows Named Mutex：

```text
Local\WorkMate.SingleInstance
```

并保留 V7.0 兼容 Mutex。第二次启动只向已存在窗口发送“显示主界面”消息，然后退出。

## 安装

正式发行使用 Inno Setup 6 工程 `installer/WorkMate.iss`。旧的自制 Setup 已从 V7.1 正式源码中移除。

## 代码签名

生产发行建议使用受 Windows 信任的 Authenticode 证书，对：

1. `WorkMate.exe`
2. `WorkMate-Setup-*.exe`

分别签名并使用 RFC3161 时间戳。具体见 `SIGNING.md`。
