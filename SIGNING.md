# WorkMate Windows 代码签名

## 生产环境应该使用什么证书

生产分发不要使用自签名证书。可选方案：

1. Microsoft Trusted Signing / Azure Artifact Signing（若你的地区和账号可用）。
2. 受 Windows 信任的 CA 签发的 Authenticode 代码签名证书（OV 等）。
3. 如果未来只通过 Microsoft Store 发布 MSIX，可由 Store 在发布流程中完成签名。

WorkMate 当前是 EXE + Inno Setup 安装包，因此最通用的方式是 Authenticode 证书 + Windows SDK 的 `signtool.exe`。

## 证书要求

- 证书应支持 Code Signing EKU。
- 新发布建议使用 RSA 证书；不要为新版本只使用 SHA-1。
- 文件摘要使用 SHA-256。
- 使用 RFC3161 时间戳并指定 SHA-256。
- 私钥不要提交进 Git，不要把 PFX 和密码放进源码或安装包。

## PFX 签名方式

先安装 Windows SDK，确保 `signtool.exe` 可用。

在命令提示符临时设置：

```bat
set WORKMATE_CERT_PFX=D:\certs\workmate-code-signing.pfx
set WORKMATE_CERT_PASSWORD=你的PFX密码
set WORKMATE_TIMESTAMP_URL=你的CA提供的RFC3161时间戳地址
scripts\sign-release.cmd
```

## 证书已经安装在 Windows Certificate Store

找到证书 SHA-1 thumbprint，然后：

```bat
set WORKMATE_CERT_THUMBPRINT=证书指纹
set WORKMATE_TIMESTAMP_URL=你的CA提供的RFC3161时间戳地址
scripts\sign-release.cmd
```

## 推荐发布顺序

先构建主程序：

```bat
scripts\build-app.cmd
```

然后设置证书环境变量并运行：

```bat
scripts\sign-release.cmd
```

这个脚本会按正确顺序自动执行：

```text
1. 签名 dist\WorkMate.exe
2. 用已签名的 WorkMate.exe 构建 Inno Setup 安装包
3. 签名 WorkMate-Setup-0.8.0.exe
4. 对两个文件执行 signtool verify
```

## 为什么一定要时间戳

时间戳可以让签名在代码签名证书过期后仍然保持可验证。正式发布不要省略时间戳。
