# deepin-power-control

Deepin power mode management component for the DDE desktop environment. It
implements the four performance modes (performance / balance / saving /
low battery) of the desktop and applies the corresponding power policy by
driving [TLP](https://linrunner.de/tlp/), the Linux power management
toolkit.

## Features

- **Four power modes**: `performance`, `balance`, `saving` and `lowbat`
  (low battery), switchable through DDE or via the command line.
- **TLP-based policy**: generates per-mode TLP configuration fragments and
  lets TLP apply CPU, disk, PCIe, USB, GPU and platform power settings.
- **Rule framework (YAML)**: mode and hardware aware rules with three
  action types:
  - `config` – emit TLP configuration keys,
  - `write` – write verified sysfs values,
  - `exec` – run a program when the rule matches.
- **Hardware feature detection**: declarative detection
  (`internal/features/features.yaml`) using cheap sysfs checks
  (`file_value_in`, `file_contains`, `path_exists`, PCI vendor/device,
  ...), plus a small code registry for cases that cannot be expressed
  declaratively.
- **OEM customization layers**: built-in rules (top level), hardware
  customization shipped by os-config (`rules/os-config/`) and direct OEM
  customization (`rules/zz-oem/`) – later layers override earlier ones.
- **DDE integration**: the desktop passes per-invocation switches (e.g.
  `--dev-idle` for device idle power saving) via the `set` command.

## Build

```bash
make build        # produces out/deepin-power-control
make test         # unit tests
```

The package also builds with `dpkg-buildpackage` (see `debian/`).

## Usage

```bash
sudo deepin-power-control set balance                 # switch mode
sudo deepin-power-control set saving --dev-idle       # + device idle saving
sudo deepin-power-control init-start                  # apply on boot
```

Run `deepin-power-control --help` for all commands and options.

## Configuration and customization

- `configs/` – base and per-mode TLP configuration files.
- `rules/` – YAML rules (`match: {mode, feature}` + `config|write|exec`).
- `internal/features/features.yaml` – hardware detection declarations.
- Customization priority (later wins): top-level rules
  (`rules/*.yaml`) < `rules/os-config/` < `rules/zz-oem/`.

## Runtime dependencies

- **TLP** (GPL-2.0-or-later) – invoked via its command line interface.
- **iw** – wireless power management support.

## License

This project is licensed under the [GNU General Public License v3.0 or
later](LICENSE).

### Third-party components

The following libraries are statically linked into the binary:

- [Cobra](https://github.com/spf13/cobra) – Apache-2.0
- [ini.v1](https://gopkg.in/ini.v1) – Apache-2.0
- [yaml.v2](https://gopkg.in/yaml.v2) – Apache-2.0
