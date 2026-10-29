<div align="center">

<picture>
  <source srcset="static/Exasol_Logo_2025_Bright.svg" media="(prefers-color-scheme: dark)">
  <img src="static/Exasol_Logo_2025_Dark.svg" alt="Exasol Logo" width="300">
</picture>

# Exasol Personal

**The Analytics Database for Agentic AI — Free for Personal Use**

*Deploy and explore Exasol Database on your own computer or in your cloud account.*

[![User documentation](https://img.shields.io/badge/user%20docs-latest-blue)](https://exasol.github.io/exasol-personal/latest/)
[![Exasol documentation](https://img.shields.io/badge/database%20docs-exasol.com-blue)](https://docs.exasol.com/db/latest/home.htm)
[![Community](https://img.shields.io/badge/community-exasol-green)](https://community.exasol.com)
[![Downloads](https://img.shields.io/badge/downloads-exasol.com-orange)](https://downloads.exasol.com/exasol-personal)

[Changelog](CHANGELOG.md)

</div>

## 📖 About Exasol Personal

Exasol is an in-memory, massively parallel analytics database with full SQL support, a vast
ecosystem, extensive AI capabilities, and flexible extensibility.

Exasol Personal gives individual users a complete Exasol database for development, exploration,
analytics, and AI-assisted workflows. It runs either locally (on macOS, Linux, or Windows) or on
one of the supported clouds (AWS, Azure, STACKIT, or Exoscale), managed by the Exasol Launcher, an
open-source scriptable command-line application. Exasol Personal is free for personal use and
does not impose an artificial data-size limit.

## 🚀 Get started

To install and run Exasol Personal locally on macOS, Linux, or Windows, follow these steps.

Install the launcher:

```bash
curl https://downloads.exasol.com/exasol-personal/installer.sh | sh
```

The installer places the `exasol` binary in `~/.local/bin`. If that directory is not in `PATH`,
follow the instructions printed by the installer.

Install and start the database:

```bash
exasol install local
```

Connect to your database:

```bash
exasol connect
```

Type any SQL statement terminated by `;`. For example, test your connection with `SELECT 1;`.
See the [Exasol SQL reference](https://docs.exasol.com/db/latest/sql_reference.htm) for the
full syntax.

For next steps or instructions on how to install Exasol Personal on your cloud infrastructure,
read the [user documentation](https://exasol.github.io/exasol-personal/latest/).

A local deployment can also serve an MCP server for AI agents:

```bash
exasol mcp enable
exasol mcp instructions claude-code
```

See [Connect an AI agent](https://exasol.github.io/exasol-personal/latest/ai-agents/) for the
clients with setup instructions.

If you work with AI coding agents, the
[Exasol Personal Local Starter Kit](https://github.com/exasol-labs/exasol-personal-local-starterkit)
bundles an MCP server, agent skills, and other components that let your agent work with Exasol
directly.

Install it with:

```bash
curl https://www.exasol.com/install/ | sh
```

## 🛠️ Development and contributions

This repository contains the Exasol Launcher, its built-in deployment presets, tests, and user
documentation sources. To build or contribute:

- Read the [contribution guidelines](CONTRIBUTING.md).
- Set up the project with the [development guide](doc/development.md).
- Follow the [testing guide](tests/README.md).
- Review the high-level [architecture](doc/architecture.md) and
  [project-specific best practices](doc/best_practices.md).

## ⚖️ License

The Exasol Launcher source code is licensed under the [MIT License](LICENSE). Exasol Database is
proprietary software provided by Exasol AG and is free for personal use. Installing the database
means accepting the
[Exasol Personal End User License Agreement](https://www.exasol.com/terms-and-conditions/#h-exasol-personal-end-user-license-agreement).

For help and discussion, visit the [Exasol Community](https://community.exasol.com) and use the
`exasol-personal` tag.
