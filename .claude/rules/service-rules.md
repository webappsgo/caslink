# Service Rules (PART 24, 25)

**These rules are NON-NEGOTIABLE. Violations are bugs.**

## PRIVILEGE ESCALATION (PART 24)
- `--service --install` installs as system service
- `--service --uninstall` removes system service
- Privilege escalation only for service install/uninstall
- Drop privileges after binding privileged ports

## SUPPORTED SERVICE MANAGERS (PART 25)
| Platform | Manager |
|----------|---------|
| Linux | systemd (primary), runit, rc.d |
| macOS | launchd |
| Windows | Windows Service Manager (SCM) |
| BSD | rc.d |

## SYSTEMD SERVICE
- Unit file: `/etc/systemd/system/caslink.service`
- Service user: `caslink` (system user, no login shell)
- Service group: `caslink`
- Restart: on-failure
- After: network.target

## SERVICE COMMANDS
```
caslink --service start
caslink --service stop
caslink --service restart
caslink --service reload
caslink --service --install
caslink --service --uninstall
caslink --service --disable
caslink --service --help
```

---
For complete details, see AI.md PART 24, 25
