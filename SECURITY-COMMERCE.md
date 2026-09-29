# 商业授权安全设计

## 防止“改注册表就永久 Pro”

Pro 身份从不读取一个 `Pro=true` 注册表值。永久 Pro 必须存在通过 Ed25519 公钥验证、且 DeviceID 与当前电脑一致的 `license.dat`。

试用期状态采用两个冗余锚点：

- `%APPDATA%\WorkMate\commerce-state.dat`
- `HKCU\Software\WorkMate\Security\TrialState`

两处内容都带完整性 MAC；应用读取后取更早的首次启动时间，并用另一份修复缺失/篡改的锚点。因此只修改某一个注册表值不会延长试用，也不能变成 Pro。

同时记录最后运行时间，明显的系统时钟回拨会终止试用。

## 本地方案的边界

没有服务器的桌面软件无法做到绝对防重置：有技术能力的用户若同时删除所有本地锚点、分析二进制或在虚拟机里重建系统，仍可能重新获得试用。

正式规模化销售时，推荐把 TrialStart / DeviceID 同步到最小授权服务器。服务器只保存授权元数据，不保存工资、加班、请假等私人数据。

## 为什么不加壳

WorkMate 不使用 UPX、自解密、自注入、隐藏 PowerShell、反调试壳等高风险手段。正式安全策略是：

- Authenticode 代码签名
- Inno Setup 标准安装
- Ed25519 授权签名
- 设备绑定
- 最小化服务器授权（未来可选）

这样对 Windows Defender 和最终用户更友好。
