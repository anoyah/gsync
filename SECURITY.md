# Security policy

## Reporting a vulnerability

Do not disclose credentials, private repository data, or an exploitable security issue in a public issue.

After this repository is uploaded to GitHub, use **Security → Advisories → Report a vulnerability** if private vulnerability reporting has been enabled by the maintainer. If that option is unavailable, ask the maintainer for a private reporting channel without posting exploit details. No security email address is currently advertised.

Include the affected version or commit, operating system, Git version, a minimal reproduction using disposable local repositories, and the potential impact. Redact URLs, access tokens, credentials and personal information. There is no guaranteed response time or paid vulnerability program.

## Scope and support

This is an early-stage project. Security fixes target the latest development version; older releases do not have a separate maintenance commitment.

gsync executes the installed Git executable, which may invoke repository hooks, SSH and credential helpers. Use trusted repositories and a trusted Git installation. Credential storage belongs to Git and the operating system. Diagnostic redaction covers HTTP(S) URL userinfo, not every possible secret in custom Git output or URL query strings.

The repository lock coordinates gsync processes, including linked worktrees. It cannot stop another Git client from changing the worktree concurrently. The macOS background service operates as the logged-in user, not as a privileged system daemon. Review the README's limitations before unattended use.
