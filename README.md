# WorkMate V8 Commerce

这是 WorkMate 的商业化基础版本，基于 V7.1.2 Native 重构。

## 新增

- 24 小时完整 Pro 试用
- Free / Pro 功能分层
- 试用到期自动弹出购买窗口
- 个人收款码入口（`payment_qr.png`）
- 设备绑定永久授权
- Ed25519 数字签名激活码
- 试用状态文件 + 注册表双锚点
- 时钟回拨检测
- 不把私钥放进客户端
- 继续兼容标准 Inno Setup / Authenticode 发布流程

## 正式发布前

1. 替换 `assets/payment_qr.png` 为你的收款码。
2. 运行 `scripts\init-license-keys.cmd` 生成你自己的授权主密钥。
3. 修改购买窗口价格/联系方式文案（默认示例 ¥19.9）。
4. `scripts\build-app.cmd`
5. 使用代码签名证书执行 `scripts\sign-release.cmd`

详见：

- `COMMERCE.md`
- `SECURITY-COMMERCE.md`
- `SIGNING.md`
