# deepin-power-control

DDE 桌面环境的电源管理模式控制组件。实现桌面的四种性能模式（性能模式 /
平衡模式 / 节能模式 / 低电量模式），并通过驱动
[TLP](https://linrunner.de/tlp/)（Linux 电源管理工具集）应用对应的电源策略。

## 功能特性

- **四种电源模式**：`performance`（性能）、`balance`（平衡）、
  `saving`（节能）、`lowbat`（低电量），可通过 DDE 或命令行切换。
- **基于 TLP 的策略**：为每种模式生成 TLP 配置片段，由 TLP 应用
  CPU、磁盘、PCIe、USB、GPU 及平台功耗设置。
- **规则框架（YAML）**：支持模式与硬件匹配的规则，动作分三种：
  - `config`：输出 TLP 配置键；
  - `write`：写入经校验的 sysfs 值；
  - `exec`：规则匹配时执行程序。
- **硬件特性检测**：声明式检测（`internal/features/features.yaml`），
  使用廉价 sysfs 检查（`file_value_in`、`file_contains`、
  `path_exists`、PCI 厂商/设备号等），并保留少量无法声明化时的代码注册表。
- **OEM 定制分层**：内置规则（顶层）、os-config 硬件定制投放
  （`rules/os-config/`）、直接 OEM 定制（`rules/zz-oem/`）——后层覆盖前层。
- **DDE 集成**：桌面通过 `set` 命令传入每次调用的开关（例如
  `--dev-idle` 设备空闲省电）。

## 构建

```bash
make build        # 生成 out/deepin-power-control
make test         # 单元测试
```

也支持 `dpkg-buildpackage` 打包（见 `debian/`）。

## 使用

```bash
sudo deepin-power-control set balance                 # 切换模式
sudo deepin-power-control set saving --dev-idle       # 节能 + 设备空闲省电
sudo deepin-power-control init-start                  # 开机应用
```

运行 `deepin-power-control --help` 查看全部命令与选项。

## 配置与定制

- `configs/`：基础与各模式的 TLP 配置文件。
- `rules/`：YAML 规则（`match: {mode, feature}` + `config|write|exec`）。
- `internal/features/features.yaml`：硬件特性检测声明。
- 定制优先级（后加载覆盖）：顶层规则（`rules/*.yaml`）<
  `rules/os-config/` < `rules/zz-oem/`。

## 运行时依赖

- **TLP**（GPL-2.0-or-later）——通过其命令行接口调用。
- **iw** —— 无线电源管理支持。

## 许可证

本项目采用 [GNU General Public License v3.0 or later](LICENSE) 许可。

### 第三方组件

以下库被静态链接进二进制：

- [Cobra](https://github.com/spf13/cobra) – Apache-2.0
- [ini.v1](https://gopkg.in/ini.v1) – Apache-2.0
- [yaml.v2](https://gopkg.in/yaml.v2) – Apache-2.0
