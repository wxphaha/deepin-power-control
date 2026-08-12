# configs/ 目录说明

本目录存放 TLP 基础配置文件（包内置，由 dpkg 安装）：

- `00-deepin-power-control-base.conf` — 所有模式的通用基础配置
- `10-deepin-power-control-mode.conf` — 模式差异配置（[performance]/[balance]/[saving]/[lowbat] 段）

## 优先级

configs/ 目录下的 .conf 文件会被按优先级读取并合并（相对路径词法序：数字前缀全局生效，后合并的覆盖先合并的）。

**注意**：configs/ 的配置可能被 rules/ 目录下的规则覆盖——rules 的 config 片段在文件系统配置**之后**合并，所以 rules 的优先级高于 configs/。OEM 定制请投放 rules/（/usr/share/deepin-power-control/rules/），不要在此目录添加 .conf 文件。

## rules/ 优先级速查

- 相对路径词法序：**数字前缀全局生效**（15-xxx.yaml < 20-intel/1.yaml < 99.yaml），目录只是归类容器
- 目录也参与排序，需带数字前缀；无前缀目录（如 oem/）排在所有数字前缀之后（oem/ 即"最后执行"层）
- 注意：词法序不解析数字，"100-xxx" 排在 "99-xxx" 之前——前缀请保持两位（00-99）
