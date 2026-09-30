# WorkMate 云服务器部署

先在本机完成开发，以后租好服务器再部署。后台、购买网页和订单数据库都放在服务器，用户只安装 Windows 客户端；你从浏览器登录管理后台，人工核实到账并开通 Pro。

## 准备

- 一台使用 systemd 的 Linux 云服务器（建议 Ubuntu / Debian）。普通 Intel/AMD 服务器使用 amd64 包，ARM 服务器使用 arm64 包。
- 一个实际域名，例如 `pro.你的域名.com`，DNS 指向服务器公网 IP。
- 服务器已安装 Caddy，安装方式见 https://caddyserver.com/docs/install 。Caddy 会为实际域名申请和续期 HTTPS 证书；服务器的 80、443 端口需要能从公网访问。后台 8090 端口只监听本机，不需要开放到公网。
- 你当前后台的整个 `admin-data`。先停止旧后台，再复制到服务器临时目录，保持原数据库和授权密钥配套。不要在服务器重新生成签名密钥。

## 生成部署包

Windows 项目根目录：

```bat
scripts\build-server.cmd
```

Linux / WSL 项目根目录：

```sh
bash scripts/build-server.sh
```

输出 `dist/WorkMate-Server-Linux-amd64.tar.gz`；传入 `arm64` 可生成 ARM 版。包内包含可运行的后台、自启动服务、HTTPS 模板、安装/备份脚本和构建客户端时使用的公钥，不包含数据库、密码或私钥。服务器无需安装 Go 或 Node。

## 第一次部署

上传部署包及旧后台的整个 `admin-data`，然后在服务器执行（将域名和目录换成自己的）：

```sh
tar -xzf WorkMate-Server-Linux-amd64.tar.gz
cd server-linux-amd64
chmod +x workmate-admin install.sh backup.sh
sudo bash install.sh pro.你的域名.com /你上传的/admin-data
```

实际安装参数请使用域名的 ASCII 形式（国际化域名使用 punycode）。安装脚本检查原密钥、公钥和数据库，用专用 `workmate` 用户运行后台，启用开机启动和异常退出重启；为 Caddy 添加站点，不覆盖其他已有站点。HTTPS 证书签发需要 DNS 生效及端口可达。第一次上线后通过浏览器确认 HTTPS 能正常访问。

访问 `https://你的实际域名/admin`。账号仍是 `admin`，密码、价格、收款码、已有订单和授权都沿用迁移的数据。

位置：

- `/opt/workmate/workmate-admin`：后台程序。
- `/var/lib/workmate`：数据库、付款资料、管理员信息和签名密钥，更新程序不会替换它。
- `/etc/workmate/admin.env`：服务器配置。
- `/etc/caddy/workmate.d/workmate.caddy`：HTTPS 站点配置。

## 客户端连接公网后台

Windows 项目根目录执行（填实际 HTTPS 域名）：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\configure-client.ps1 -ServerUrl https://pro.你的域名.com
```

它只修改 `assets/commerce.json` 的 `server_url`，保留价格和其他购买配置。然后执行 `scripts\build-app.cmd`，将新版客户端发给用户。服务器必须继续使用与客户端公钥对应的原始后台密钥。

已经发出的免安装版客户端，也可直接修改 EXE 同目录的 `commerce.json` 的 `server_url` 并重启；或者运行上面的脚本并加 `-ConfigPath C:\客户端目录\commerce.json`。只改地址不需要重新编译或换密钥。已经永久激活的授权不会因地址变化失效。

最新版客户端下载包也附带 `configure-client.ps1`。在解压目录执行 `powershell -NoProfile -ExecutionPolicy Bypass -File .\configure-client.ps1 -ServerUrl https://你的实际域名`，会自动找到同目录的 `commerce.json`。

原本在本机测试地址创建的待处理订单访问凭证绑定旧地址；迁移数据不等于自动修改客户端缓存。正式分发前配置好公网地址；本机测试留下的未完成订单可在原后台处理，或给用户提供对应已签发的备用激活码。

## 更新与备份

更新时上传新版包，解压后执行，不传数据目录参数：

```sh
sudo bash install.sh 你的实际域名
```

检测到已有数据库时，脚本保留整个数据目录；带上另一个数据目录会拒绝覆盖。签名密钥丢失时恢复原始密钥，不要重建。

备份：

```sh
sudo bash backup.sh /你自己的备份目录
```

脚本短暂停止后台，备份整个数据目录，然后恢复原来运行的服务。备份含签名私钥及订单资料，只由你保存。恢复时停止后台、解压备份到 `/var/lib`，恢复所有者 `workmate:workmate` 后启动。

检查运行状态与日志：

```sh
sudo systemctl status workmate-admin
sudo journalctl -u workmate-admin -n 100
curl --fail https://你的实际域名/healthz
```

## 服务器配置

命令行参数优先于环境变量，本机默认地址不变：

| 环境变量 | 默认值 | 作用 |
| --- | --- | --- |
| `WORKMATE_LISTEN` | `127.0.0.1:8090` | HTTP 监听地址 |
| `WORKMATE_DATA_DIR` | `admin-data` | 数据与签名密钥目录 |
| `WORKMATE_PUBLIC_URL` | 空 | 实际 HTTPS 访问地址，不包含路径 |
| `WORKMATE_DOWNLOAD_URL` | 空 | 可选：HTTPS 安装包地址，用于 `/download` 点击统计 |
| `WORKMATE_TRUSTED_PROXIES` | 空 | 可信代理 IP/CIDR，逗号分隔 |

对应参数为 `-listen`、`-data-dir`、`-public-url`、`-download-url`、`-trusted-proxies`。部署模板只信任本机 Caddy，并由 Caddy 覆盖用户发送的转发头。后台按真实来源地址限流，不会把所有用户算作同一个代理用户；未配置可信代理时忽略转发头。

数据库当前使用单机 SQLite，先运行一个后台实例。无需另买数据库服务。所有工资、加班和请假记录仍留在用户本机，后台还会统计**明确同意设备使用统计**的客户端活跃、试用、版本与固定错误类别；不会收集工资、加班和请假记录。下载量仅在配置下载入口后显示入口点击次数，不能确认安装包下载完成。

运营概览的指标口径、用户选择统计的方式及下载入口配置见 [ADMIN.md](ADMIN.md)。
