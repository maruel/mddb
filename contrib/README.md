# mddb user installation

The installer downloads the latest checksummed release and starts mddb's user
service. Run the same command to update; existing configuration, settings and
data are preserved. The installer takes no arguments.

```sh
curl -fsSL https://raw.githubusercontent.com/maruel/mddb/main/contrib/install.sh -o /tmp/install-mddb.sh
bash /tmp/install-mddb.sh
```

The binary is `~/.local/bin/mddb`. Configuration is
`~/.config/mddb/config.toml`, or `$XDG_CONFIG_HOME/mddb/config.toml` when set.
Mutable settings stay beside the configuration; content lives in `~/mddb/data`.
See [config.toml](config.toml) for settings.

Stable releases update daily at 04:50 local time, with up to five minutes of
random delay, then restart gracefully. Set `auto_update = ""` in config.toml
to disable automatic updates. Development builds stay under your control.

| Platform | User service              | Logs                                |
| -------- | ------------------------- | ----------------------------------- |
| Linux    | systemd `mddb.service`    | `journalctl --user -u mddb.service` |
| macOS    | launchd `com.maruel.mddb` | `~/Library/Logs/mddb/`              |

For other installation workflows, [build from source](../SELF_HOSTING.md#from-source).
[.goreleaser.yaml](../.goreleaser.yaml) defines amd64 and arm64 archives for
Linux, macOS and Windows. Windows zip files contain the executable and example
configuration for manual setup. `make test` validates packaging; a `v*` tag publishes a release.
